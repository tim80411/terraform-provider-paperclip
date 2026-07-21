// internal/provider/secret_resource.go
// paperclip_company_secret resource: declarative CRUD + import over the
// /api/companies/{cid}/secrets + /api/secrets/{id}(+/rotate) endpoints.
package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tim80411/terraform-provider-paperclip/internal/client"
)

// paperclipManagedMode is the only managedMode this resource ever writes.
// live 探測（context pack §欄位參考）：正確值是 "paperclip_managed"，不是 spec 草稿猜的 "inline"。
const paperclipManagedMode = "paperclip_managed"

type secretResource struct {
	client *client.Client
}

func NewSecretResource() resource.Resource { return &secretResource{} }

type secretResourceModel struct {
	ID           types.String `tfsdk:"id"`
	CompanyID    types.String `tfsdk:"company_id"`
	Name         types.String `tfsdk:"name"`
	Key          types.String `tfsdk:"key"`
	Value        types.String `tfsdk:"value"`
	ValueVersion types.String `tfsdk:"value_version"`
}

func (r *secretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_company_secret"
}

func (r *secretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"company_id": schema.StringAttribute{
				Required:      true,
				Description:   "父 company 的 id。secret 不能跨 company 搬動，改這個欄位會整個重建。",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true},
			"key": schema.StringAttribute{
				Required: true,
				Description: "secret 的識別 key。live 探測：paperclip 會把 key 正規化成小寫" +
					`（例如 "GH_TOKEN" 存回 "gh_token"）。本 provider 在 state 保留你 config 寫的` +
					"原始大小寫，避免每次 plan 都因為這個正規化顯示假 diff。匯入（terraform import）情境" +
					"例外：import 沒有 prior state 可比對大小寫，一律直接採用 API 回傳的（已小寫化）值——" +
					`匯入一個 config 用大寫 key（如 "GH_TOKEN"）宣告的既有 secret，state 會落地成小寫的` +
					`"gh_token"，跟 config 大小寫不同；之後每次 plan 都要把 config 改成小寫，或接受這個` +
					"已知落差。",
			},
			"value": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				Description: "secret 明文值。live 探測：API 完全不會回傳它（連 hasValue 欄位都沒有）" +
					"——Read 永不覆寫這裡，只保留 config/prior 值。單獨改這個欄位不會觸發 rotate；" +
					"要送新值到 paperclip，必須同時 bump value_version。",
			},
			"value_version": schema.StringAttribute{
				Optional: true,
				Description: "改這個值來觸發 rotate（呼叫 POST /api/secrets/{id}/rotate，" +
					"送出目前 config 的 value）。只改 value 不 bump 這個欄位，state 會更新但不會打 rotate。",
			},
		},
	}
}

func (r *secretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected ProviderData", fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *secretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan secretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in := client.SecretCreateInput{
		Name:        plan.Name.ValueString(),
		Key:         plan.Key.ValueString(),
		Value:       plan.Value.ValueString(),
		ManagedMode: paperclipManagedMode,
	}
	got, err := r.client.CreateSecret(ctx, plan.CompanyID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Create secret failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, secretFromAPI(got, plan))...)
}

func (r *secretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetSecret(ctx, state.CompanyID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read secret failed", err.Error())
		return
	}
	model := secretFromAPI(got, state)
	model.Key = reconcileKey(state.Key, got.Key)
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *secretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state secretResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 擋掉「改了 value 卻沒 bump value_version」——這個組合如果放行，Update 會把新 value
	// 寫進 state 但完全不打 rotate，live 上的 secret 仍是舊值，且因為 state 已經跟 plan
	// 一致，之後也不會再有任何 plan diff 讓人發現漏了 rotate。寧可當場報錯，逼使用者
	// 一起 bump value_version 來觸發真正的 rotate。
	if valueChangedWithoutVersionBump(plan, state) {
		resp.Diagnostics.AddError(
			"value changed but value_version was not bumped",
			"`value` differs from the current state but `value_version` did not change, so this secret "+
				"would silently keep its old value live while state moved on as if it rotated. "+
				"Bump `value_version` (to any new value) in the same change to actually rotate the secret.",
		)
		return
	}

	// 只送有變的欄位（指標）→ 保留未管欄位（spec §6.3）。
	in := buildSecretUpdateInput(plan, state)
	var got *client.Secret
	var err error
	if in.Name != nil || in.Key != nil {
		got, err = r.client.UpdateSecret(ctx, state.ID.ValueString(), in)
		if err != nil {
			resp.Diagnostics.AddError("Update secret failed", err.Error())
			return
		}
	}

	// rotate 只在 value_version 改變時才打——單獨改 value 不會觸發（brief 明訂，也是
	// spec §6.2 write-only secret 的標準手法：value 讀不回來，version 才是漂移偵測的錨點）。
	if shouldRotateSecret(plan, state) {
		got, err = r.client.RotateSecret(ctx, state.ID.ValueString(), plan.Value.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Rotate secret failed", err.Error())
			return
		}
	}

	if got == nil {
		// name/key/value_version 都沒變（上面的 guard 已經擋掉「只改 value」的情況，
		// 所以真的走到這裡代表整個 model 跟 state 一致）——不需要打 PATCH 或 rotate，
		// id/name 這些欄位也早就跟 state 一樣，不必為了組 state 多打一次 GetSecret。
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, secretFromAPI(got, plan))...)
}

func (r *secretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSecret(ctx, state.ID.ValueString()); err != nil && !client.IsGone(err) {
		resp.Diagnostics.AddError("Delete secret failed", err.Error())
	}
}

// ImportState 用複合 ID "<company_id>/<secret_id>"：live 探測沒有 GET /api/secrets/{id}
// 單筆端點（回 404 "API route not found"），GetSecret 得靠 list-under-company 實作，
// 因此光有 secret id 不夠，import 時必須一起帶 company_id。
func (r *secretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected Import Identifier",
			fmt.Sprintf(`Expected import ID in the form "company_id/secret_id", got: %q`, req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("company_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// buildSecretUpdateInput returns an update input containing ONLY the fields
// that changed between state and plan (spec §6.3). value/value_version never
// appear here — they are rotate's job, never PATCH's (§6.2).
func buildSecretUpdateInput(plan, state secretResourceModel) client.SecretUpdateInput {
	var in client.SecretUpdateInput
	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		in.Name = &v
	}
	if !plan.Key.Equal(state.Key) {
		v := plan.Key.ValueString()
		in.Key = &v
	}
	return in
}

// shouldRotateSecret reports whether value_version changed between state and
// plan — the sole trigger for POST /api/secrets/{id}/rotate. A bare change to
// `value` without bumping `value_version` is intentionally NOT a trigger.
func shouldRotateSecret(plan, state secretResourceModel) bool {
	return !plan.ValueVersion.Equal(state.ValueVersion)
}

// valueChangedWithoutVersionBump reports the one write-only-secret footgun
// Update() refuses to silently accept: `value` changed but `value_version`
// didn't move with it. shouldRotateSecret would return false in that case, so
// Update() would neither PATCH nor rotate — yet without this guard it would
// still happily write the new `value` into state, making the live secret's
// drift from state permanent and invisible to any future plan.
func valueChangedWithoutVersionBump(plan, state secretResourceModel) bool {
	return !plan.Value.Equal(state.Value) && plan.ValueVersion.Equal(state.ValueVersion)
}

// reconcileKey preserves the caller's own casing for `key` unless the API's
// value is a genuine (non-case) drift. live 探測：paperclip 把 key 正規化成小寫
// （"GH_TOKEN" → "gh_token"）；若逐字比對 API 回傳值會讓每次 Read 都出現假 diff，
// 所以只有大小寫以外的差異才代表 key 真的被外部改掉，這時才採用 API 的值。
func reconcileKey(prior types.String, apiKey string) types.String {
	if !prior.IsNull() && strings.EqualFold(prior.ValueString(), apiKey) {
		return prior
	}
	return types.StringValue(apiKey)
}

// secretFromAPI builds resource state from the API response, keeping the
// caller's own copy of fields the API either never returns (`value`,
// write-only) or silently normalizes (`key`'s casing — see reconcileKey).
// Create/Update always pass `plan` as base so the returned state matches
// exactly what was planned (required for non-Computed attributes); Read
// passes `state` and then applies reconcileKey on top.
func secretFromAPI(got *client.Secret, base secretResourceModel) secretResourceModel {
	base.ID = types.StringValue(got.ID)
	base.Name = types.StringValue(got.Name)
	return base
}

var (
	_ resource.Resource                = &secretResource{}
	_ resource.ResourceWithConfigure   = &secretResource{}
	_ resource.ResourceWithImportState = &secretResource{}
)
