package app.test_abac

import rego.v1

import data.app.abac as policy

test_admin_positive if {
	inp := {
		"user": "alice",
		"action": "read",
		"resource": "dog123",
	}

	policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

test_senior_employee_positive if {
	inp := {
		"user": "bob",
		"action": "update",
		"resource": "dog123",
	}

	policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

test_junior_employee_positive if {
	inp := {
		"user": "eve",
		"action": "read",
		"resource": "dog123",
	}

	policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

test_junior_employee_negative if {
	inp := {
		"user": "eve",
		"action": "update",
		"resource": "dog123",
	}

	not policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

test_customer_positive if {
	inp := {
		"user": "dave",
		"action": "read",
		"resource": "dog456",
	}

	policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

test_customer_negative if {
	inp := {
		"user": "dave",
		"action": "read",
		"resource": "dog123",
	}

	not policy.allow with input as inp
		with data.user_attributes as user_attributes
		with data.pet_attributes as pet_attributes
}

# Attributes of users
#   Hardcoded in policy in this example, but in reality would be treated as external data.
user_attributes := {
	"alice": {"tenure": 20, "title": "owner"},
	"bob": {"tenure": 15, "title": "employee"},
	"eve": {"tenure": 5, "title": "employee"},
	"dave": {"tenure": 5, "title": "customer"},
}

# Pet attributes
#   Hardcoded in policy in this example, but in reality would be treated as external data.
pet_attributes := {
	"dog123": {"adopted": true, "age": 2, "breed": "terrier", "name": "toto"},
	"dog456": {"adopted": false, "age": 3, "breed": "german-shepherd", "name": "rintintin"},
	"dog789": {"adopted": false, "age": 2, "breed": "collie", "name": "lassie"},
	"cat123": {"adopted": false, "age": 1, "breed": "fictitious", "name": "cheshire"},
}
