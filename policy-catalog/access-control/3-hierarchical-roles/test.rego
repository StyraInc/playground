package app.rbac.hierarchical_test

import rego.v1

import data.app.rbac.hierarchical as policy

test_alice_allowed if {
	inp := {
		"user": "alice",
		"action": "delete-project",
		"roles": ["tech-lead"],
	}

	policy.allow with input as inp
		with data.permissions as sample_data.permissions
		with data.roles_graph as sample_data.roles_graph
}

test_bob_allowed if {
	inp := {
		"user": "bob",
		"action": "delete-project",
		"roles": ["junior-developer"],
	}

	not policy.allow with input as inp
		with data.permissions as sample_data.permissions
		with data.roles_graph as sample_data.roles_graph
}

sample_data := {
	"permissions": {
		"admin": [],
		"tech-manager": [
			"create-user",
			"delete-user",
			"update-user",
		],
		"tech-lead": ["read-user"],
		"developer": [
			"create-project",
			"delete-project",
			"update-project",
			"read-project",
		],
		"devops": [
			"update-deployment",
			"read-deployment",
		],
		"platform-engineer": [
			"create-deployment",
			"delete-deployment",
		],
		"junior-developer": [
			"read-project",
			"update-project",
		],
	},
	"roles_graph": {
		"admin": ["tech-manager"],
		"tech-manager": ["tech-lead"],
		"tech-lead": [
			"developer",
			"devops",
			"platform-engineer",
		],
		"developer": ["junior-developer"],
		"platform-engineer": ["devops"],
		"devops": [],
		"junior-developer": [],
	},
}
