import makeId from '../../dom/make-id'

import Parser from '../parser'
import * as Ast from '../ast'

function AstImport(path, alias, location) {
  this.id = makeId(AstImport.TYPE)

  this.path = path
  this.alias = alias
  this.location = location
}

AstImport.TYPE = 'import'

AstImport.parse = function (source) {
  return Parser.parse(source, {startRule: 'ImportOnly'})
}

AstImport.makePlaceholder = function () {
  const path = Ast.Reference.makePlaceholder()
  const placeholder = new AstImport(path, undefined, Ast.makeLocation())

  placeholder.isPlaceholder = true

  return placeholder
}

AstImport.prototype.clone = function () {
  const clone = new AstImport(this.path, this.alias, this.location)

  if (this.isPlaceholder === true) {
    clone.isPlaceholder = true
  }

  return clone
}

AstImport.prototype.getId = function () {
  return Ast.Reference.stringify(this.path.value)
}

AstImport.prototype.getName = function () {
  return this.alias ? `${this.alias}` : this.getRealName()
}

AstImport.prototype.getRealName = function () {
  const path = this.path.value

  if (path.length < 2) {
    return ''
  }

  return `${path[path.length - 1]}`
}

AstImport.prototype.toJSON = function () {
  const {path, alias, location} = this
  return {
    path,
    alias: alias ? String(alias) : undefined,
    location
  }
}

AstImport.prototype.toString = function () {
  return `import ${this.getId()}${this.alias ? ` as ${this.alias}` : ''}`
}

export default AstImport
