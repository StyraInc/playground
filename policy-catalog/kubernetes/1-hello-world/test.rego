package kubernetes.validating.test_label

import rego.v1

import data.kubernetes.validating.label as policy

test_validating_label_positive if {
	actual := policy.deny with input as {"request": {"object": {"metadata": {"labels": {"costcenter": "foo"}}}}}

	# actual should be a set with 1 error message
	actual == {"Costcenter code must start with `cccode-`; found `foo`"}
}

test_validating_label_negative if {
	actual := policy.deny with input as {"request": {"object": {"metadata": {"labels": {"costcenter": "cccode-foo"}}}}}
	count(actual) == 0
}
