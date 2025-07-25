import * as ast from '../ast'

function AstModule() {
  this.package = null
  this.imports = []
  this.rules = []
  this.comments = []
}

AstModule.TYPE = 'module'

AstModule.prototype.addStatement = function (statement) {
  const type = statement.constructor

  if (type === ast.Comment) {
    this.comments.push(statement)
  } else if (type === ast.Rule) {
    this.rules.push(statement)
  } else if (type === ast.Import) {
    this.imports.push(statement)
  } else if (type === ast.Package) {
    this.package = statement
  } else {
    throw new TypeError(`Grammar out-of-sync: ${String(type)} ${JSON.stringify(statement)}`)
  }
}

AstModule.prototype.toJSON = function () {
  return {
    package: this.package,
    imports: unlessEmpty(this.imports),
    rules: unlessEmpty(this.rules),
    comments: unlessEmpty(this.comments)
  }
}

export default AstModule

// -----------------------------------------------------------------------------

function unlessEmpty(array) {
  return array.length > 0 ? array : undefined
}
