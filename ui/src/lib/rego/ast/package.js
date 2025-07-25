import makeId from '../../dom/make-id'

import * as Ast from '../ast'

function AstPackage(path, location) {
  this.id = makeId(AstPackage.TYPE)

  // All packages are implicitly declared under the default root.
  this.path = new Ast.Reference(Ast.Variable.DEFAULT_ROOT)
  this.location = location

  const type = path.constructor
  const value = path.value

  if (type === Ast.Reference) {
    for (let i = 0, ni = value.length; i < ni; ++i) {
      const node = value[i]
      this.path.value.push(new Ast.String(node.value, node.location))
    }
  } else if (type === Ast.Variable) {
    this.path.value.push(new Ast.String(value, path.location))
  }
}

AstPackage.TYPE = 'package'

AstPackage.prototype.getId = function () {
  // Omit `this.path[0]` -- all packages are implicitly declared under the
  // default root.
  return Ast.Reference.stringify(this.path.value.slice(1))
}

AstPackage.prototype.getName = function () {
  const path = this.path.value
  return String(path[path.length - 1])
}

AstPackage.prototype.toJSON = function () {
  return {
    path: this.path.value.slice(),
    location: this.location
  }
}

AstPackage.prototype.toString = function () {
  return `package ${this.getId()}`
}

export default AstPackage
