# Ingress Conflicts
# -----------------
#
# This example prevents conflicting Kubernetes Ingresses from being created. Two
# Kubernetes Ingress resources are considered in conflict if they have the same
# hostname. This example shows how to:
#
#	* Iterate/search across JSON arrays and objects.
#	* Leverage external context in decision-making.
#	* Define helper rules that provide useful abstractions.
#
# For additional information see:
#
#	* Rego Iteration: https://www.openpolicyagent.org/docs/latest/#iteration
#	* Context-aware Admission Control Policies:
#     https://www.openpolicyagent.org/docs/latest/kubernetes-primer/#using-context-in-policies
#	* Caching structure with 'kube-mgmt': https://github.com/open-policy-agent/kube-mgmt#caching
#
# Hint: When you click Evaluate, you see values for `deny` as well as `input_ns`,
# `input_name`, and `is_ingress` because by default the playground evaluates all
# of the rules in the current package. You can evaluate specific rules by selecting
# the rule name (e.g., "deny") and clicking Evaluate Selection.

package kubernetes.validating.ingress

import rego.v1

deny contains msg if {
	# This rule only applies to Kubernetes Ingress resources.
	is_ingress

	# Find hostnames in the Kubernetes Ingress resource being validated. In Rego,
	# `_` is syntactic sugar for a unique variable. Since we do not refer to this
	# array index anywhere else, we can use `_` instead of inventing a new variable name.
	some rule in input.request.object.spec.rules
	input_host := rule.host

	# Find hostnames of other Kubernetes Ingress resources that exist in the cluster.
	# The statement `data.kubernetes.ingress[other_ns][other_name]` iterates over all
	# Kubernetes Ingress resources cached inside of OPA and assigns the hostname to the
	# variable `other_host`.
	some other_ns, other_name
	other_host := data.kubernetes.ingresses[other_ns][other_name].spec.rules[_].host

	# Check if this Kubernetes Ingress resource is the same as the other one that
	# exists in the cluster. This is important because this policy will be applied
	# to CREATE and UPDATE operations. Resources do not conflict with themselves.
	#
	# Hint: This statement constructs two arrays and then compares them. This is
	# effectively a logical OR. E.g., if `input_ns` is not equal to `other_ns` OR
	# `input_name` is not equal to `other_name` the statement is TRUE.
	[input_ns, input_name] != [other_ns, other_name]

	# Check if there is a conflict. This check could be more sophisticated if needed.
	input_host == other_host

	# Construct an error message to return to the user.
	msg := sprintf("Ingress host conflicts with ingress %v/%v", [other_ns, other_name])
}

input_ns := input.request.object.metadata.namespace

input_name := input.request.object.metadata.name

is_ingress if {
	input.request.kind.kind == "Ingress"
	input.request.kind.group == "extensions"
	input.request.kind.version == "v1beta1"
}
