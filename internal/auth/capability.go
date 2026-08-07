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
