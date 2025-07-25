package kubernetes.validating.test_images

import rego.v1

import data.kubernetes.validating.images as policy

test_validating_images_positive if {
	actual := policy.deny with input as {"request": {
		"kind": {"kind": "Pod"},
		"object": {"spec": {"containers": [
			{"image": "hooli.com/nginx"},
			{"image": "busybox"},
		]}},
	}}

	# actual should be a set with 1 error message
	actual == {"Image 'busybox' comes from untrusted registry"}
}

test_validating_images_negative if {
	actual := policy.deny with input as {"request": {
		"kind": {"kind": "Pod"},
		"object": {"spec": {"containers": [
			{"image": "hooli.com/nginx"},
			{"image": "hooli.com/busybox"},
		]}},
	}}

	count(actual) == 0
}
