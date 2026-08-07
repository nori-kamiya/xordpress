package auth

// Capability is a bitflag representing a single permission.
type Capability uint16

const (
	CapNone      Capability = 0
	CapPostsRead Capability = 1 << iota
	CapPostsEditOwn
	CapPostsEditOthers
	CapPostsPublish
	CapUsersManage
	CapSettingsManage
	CapSelfManage // アカウント設定・2FA・自分のセッション一覧/失効
)

// Role identifies a user's role for capability-based authorization.
type Role string

const (
	RoleAdministrator Role = "administrator"
	RoleEditor        Role = "editor"
	RoleAuthor        Role = "author"
	RoleContributor   Role = "contributor"
)

var roleCaps = map[Role]Capability{
	RoleAdministrator: CapPostsRead | CapPostsEditOwn | CapPostsEditOthers | CapPostsPublish | CapUsersManage | CapSettingsManage | CapSelfManage,
	RoleEditor:        CapPostsRead | CapPostsEditOwn | CapPostsEditOthers | CapPostsPublish | CapSelfManage,
	RoleAuthor:        CapPostsRead | CapPostsEditOwn | CapPostsPublish | CapSelfManage,
	RoleContributor:   CapPostsRead | CapPostsEditOwn | CapSelfManage,
}

// Has reports whether the role has the given capability. Unknown roles have
// no entry in roleCaps, so the zero-value map lookup returns CapNone and Has
// always returns false — fail closed.
func (r Role) Has(c Capability) bool { return roleCaps[r]&c == c }

// IsValidRole reports whether r is one of the four known roles, so callers
// accepting a role string from a request body (e.g. POST/PATCH /api/users)
// can reject unknown values with 400 instead of persisting a role that maps
// to no capabilities.
func IsValidRole(r Role) bool {
	_, ok := roleCaps[r]
	return ok
}

// capabilityNames pairs every Capability bit with its wire name, in a fixed
// order, so GET /api/account (design.md §5) can render a Role's
// capabilities as a stable, human-readable string array instead of exposing
// the internal bitflag.
var capabilityNames = []struct {
	cap  Capability
	name string
}{
	{CapPostsRead, "posts:read"},
	{CapPostsEditOwn, "posts:edit_own"},
	{CapPostsEditOthers, "posts:edit_others"},
	{CapPostsPublish, "posts:publish"},
	{CapUsersManage, "users:manage"},
	{CapSettingsManage, "settings:manage"},
	{CapSelfManage, "self:manage"},
}

// CapabilityNames returns the wire names of every capability r has, in
// capabilityNames' fixed order.
func (r Role) CapabilityNames() []string {
	names := []string{}
	for _, cn := range capabilityNames {
		if r.Has(cn.cap) {
			names = append(names, cn.name)
		}
	}
	return names
}
