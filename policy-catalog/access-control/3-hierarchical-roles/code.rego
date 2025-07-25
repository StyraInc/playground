# Hierarchical Access Control with Role Inheritance
# -------------------------------------------------
#
# A common method to address RBAC's problem of 'Role Explosion' is to use
# hierarchical roles. In this example, we show how to implement a simple
# hierarchical access control policy using a graph of related roles.
#
# Users make requests with one or more roles. The policy checks if the user
# has the requested permission by traversing the role hierarchy. If the user
# has the permission, the policy allows the request.
#
# Try changing the input to see how the policy behaves with these examples:
#
# Alice, the tech lead
# {
#     "user": "alice",
#     "action": "delete-project",
#     "roles": [
#         "tech-lead"
#     ]
# }
# Bob, the junior developer
# {
#     "user": "bob",
#     "action": "delete-project",
#     "roles": [
#         "junior-developer"
#     ]
# }
# Alice will inherit the permissions of the developer role, which has permissions
# to delete projects. Bob, on the other hand, will not have the permission to
# delete projects as he only has the permissions of the junior developer role
# which does not include the delete-project permission.

package app.rbac.hierarchical

import rego.v1

reachable_roles := graph.reachable(data.roles_graph, input.roles)

user_permissions contains permission if {
	some role in reachable_roles
	some permission in data.permissions[role]
}

default allow := false

allow if input.action in user_permissions
