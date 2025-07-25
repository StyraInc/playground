package kubernetes.validating.test_ingress

import rego.v1

import data.kubernetes.validating.ingress as policy

test_validating_ingress_positive if {
	# admission control request
	inp := {"request": {
		"kind": {
			"group": "extensions",
			"kind": "Ingress",
			"version": "v1beta1",
		},
		"operation": "CREATE",
		"userInfo": {
			"groups": null,
			"username": "alice",
		},
		"object": {
			"metadata": {
				"namespace": "retail",
				"name": "bar",
			},
			"spec": {"rules": [{
				"host": "initech.com",
				"http": {"paths": [{
					"path": "/finance",
					"backend": {
						"serviceName": "banking",
						"servicePort": 443,
					},
				}]},
			}]},
		},
	}}

	# existing ingress that is cached in OPA
	existing_ingress := {
		"kind": "Ingress",
		"metadata": {
			"name": "foo",
			"namespace": "ecommerce",
		},
		"spec": {"rules": [{
			"host": "initech.com",
			"http": {"paths": [{
				"path": "/finance",
				"backend": {
					"serviceName": "banking",
					"servicePort": 443,
				},
			}]},
		}]},
	}

	actual := policy.deny with input as inp
		with data.kubernetes.ingresses.ecommerce.foo as existing_ingress

	actual == {"Ingress host conflicts with ingress ecommerce/foo"}
}

test_validating_ingress_negative if {
	# admission control request
	inp := {"request": {
		"kind": {
			"group": "extensions",
			"kind": "Ingress",
			"version": "v1beta1",
		},
		"operation": "CREATE",
		"userInfo": {
			"groups": null,
			"username": "alice",
		},
		"object": {
			"metadata": {
				"namespace": "retail",
				"name": "bar",
			},
			"spec": {"rules": [{
				"host": "hooli.com",
				"http": {"paths": [{
					"path": "/finance",
					"backend": {
						"serviceName": "banking",
						"servicePort": 443,
					},
				}]},
			}]},
		},
	}}

	# existing ingress that is cached in OPA
	existing_ingress := {
		"kind": "Ingress",
		"metadata": {
			"name": "foo",
			"namespace": "ecommerce",
		},
		"spec": {"rules": [{
			"host": "initech.com",
			"http": {"paths": [{
				"path": "/finance",
				"backend": {
					"serviceName": "banking",
					"servicePort": 443,
				},
			}]},
		}]},
	}

	actual := policy.deny with input as inp
		with data.kubernetes.ingresses.ecommerce.foo as existing_ingress

	count(actual) == 0
}
