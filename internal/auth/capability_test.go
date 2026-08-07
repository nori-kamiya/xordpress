package auth_test

import (
	"testing"

	"github.com/nori-kamiya/xordpress/internal/auth"
)

func TestRole_Has(t *testing.T) {
	allCaps := []auth.Capability{
		auth.CapPostsRead,
		auth.CapPostsEditOwn,
		auth.CapPostsEditOthers,
		auth.CapPostsPublish,
		auth.CapUsersManage,
		auth.CapSettingsManage,
		auth.CapSelfManage,
	}

	want := map[auth.Role]map[auth.Capability]bool{
		auth.RoleAdministrator: {
			auth.CapPostsRead:       true,
			auth.CapPostsEditOwn:    true,
			auth.CapPostsEditOthers: true,
			auth.CapPostsPublish:    true,
			auth.CapUsersManage:     true,
			auth.CapSettingsManage:  true,
			auth.CapSelfManage:      true,
		},
		auth.RoleEditor: {
			auth.CapPostsRead:       true,
			auth.CapPostsEditOwn:    true,
			auth.CapPostsEditOthers: true,
			auth.CapPostsPublish:    true,
			auth.CapUsersManage:     false,
			auth.CapSettingsManage:  false,
			auth.CapSelfManage:      true,
		},
		auth.RoleAuthor: {
			auth.CapPostsRead:       true,
			auth.CapPostsEditOwn:    true,
			auth.CapPostsEditOthers: false,
			auth.CapPostsPublish:    true,
			auth.CapUsersManage:     false,
			auth.CapSettingsManage:  false,
			auth.CapSelfManage:      true,
		},
		auth.RoleContributor: {
			auth.CapPostsRead:       true,
			auth.CapPostsEditOwn:    true,
			auth.CapPostsEditOthers: false,
			auth.CapPostsPublish:    false,
			auth.CapUsersManage:     false,
			auth.CapSettingsManage:  false,
			auth.CapSelfManage:      true,
		},
	}

	for role, capWant := range want {
		for _, cap := range allCaps {
			got := role.Has(cap)
			if got != capWant[cap] {
				t.Errorf("Role(%q).Has(%d) = %v, want %v", role, cap, got, capWant[cap])
			}
		}
	}
}

func TestRole_Has_UnknownRoleIsFailClosed(t *testing.T) {
	unknown := auth.Role("superadmin")

	allCaps := []auth.Capability{
		auth.CapPostsRead,
		auth.CapPostsEditOwn,
		auth.CapPostsEditOthers,
		auth.CapPostsPublish,
		auth.CapUsersManage,
		auth.CapSettingsManage,
		auth.CapSelfManage,
	}

	for _, cap := range allCaps {
		if unknown.Has(cap) {
			t.Errorf("unknown role %q: Has(%d) = true, want false (fail closed)", unknown, cap)
		}
	}
}
