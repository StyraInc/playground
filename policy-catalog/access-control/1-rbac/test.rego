package app.test_rbac

import rego.v1

import data.app.rbac as policy

test_admin_positive if {
	inp := {
		"user": "alice",
		"action": "read",
		"object": "id123",
		"type": "dog",
	}

	policy.allow with input as inp
		with data.user_roles as user_roles
		with data.role_grants as role_grants
}

test_nonadmin_positive if {
	inp := {
		"user": "bob",
		"action": "read",
		"object": "id123",
		"type": "finance",
	}

	policy.allow with input as inp
		with data.user_roles as user_roles
		with data.role_grants as role_grants
}

test_nonadmin_negative if {
	inp := {
		"user": "eve",
		"action": "update",
		"object": "id123",
		"type": "dog",
	}

	not policy.allow with input as inp
		with data.user_roles as user_roles
		with data.role_grants as role_grants
}

user_roles := {
	"alice": ["admin"],
	"bob": ["employee", "billing"],
	"eve": ["customer"],
}

role_grants := {
	"customer": [
		{
			"action": "read",
			"type": "dog",
		},
		{
			"action": "read",
			"type": "cat",
		},
		{
			"action": "adopt",
			"type": "dog",
		},
		{
			"action": "adopt",
			"type": "cat",
		},
	],
	"employee": [
		{
			"action": "read",
			"type": "dog",
		},
		{
			"action": "read",
			"type": "cat",
		},
		{
			"action": "update",
			"type": "dog",
		},
		{
			"action": "update",
			"type": "cat",
		},
	],
	"billing": [
		{
			"action": "read",
			"type": "finance",
		},
		{
			"action": "update",
			"type": "finance",
		},
	],
}
