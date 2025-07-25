import makeId from '../../dom/make-id'

import * as Ast from '../ast'

function AstWith(target, value, location) {
  this.id = makeId(AstWith.TYPE)

  this.target = target
  this.value = value
  this.location = location
}

AstWith.TYPE = 'with'

AstWith.makePlaceholder = function () {
  const {with: [placeholder]} = Ast.Expression.parse('x with input.x as "x"')
  placeholder.isPlaceholder = true
  return placeholder
}

AstWith.prototype.clone = function () {
  const {target, value, location} = this
  return new AstWith(target, value, Ast.makeLocation(location))
}

AstWith.prototype.toJSON = function () {
  const {target, value, location} = this
  return {
    target,
    value,
    location
  }
}

export default AstWith
