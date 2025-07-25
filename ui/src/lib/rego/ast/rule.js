import makeId from '../../dom/make-id'

import Parser from '../parser'
import * as Ast from '../ast'

function AstRule(head, body, location) {
  this.id = makeId(AstRule.TYPE)

  this.default = false
  this.head = head
  this.body = body
  this.else = null
  this.location = location
}

AstRule.TYPE = 'rule'

// TODO: Derive .Kind from RuleHead.Kind (and RuleBody.Kind)?
AstRule.Kind = {
  COMPLETE: 'complete',
  CONSTANT: 'constant',
  FUNCTION: 'function',
  PARTIAL_OBJECT: 'object',
  PARTIAL_SET: 'set'
}

AstRule.makePlaceholder = function (head = 'comstyraplaceholder', body = 1) {
  const placeholder = AstRule.parse(`${head} { 1 }`)

  placeholder.body.remove(0)
  placeholder.body.add(0, Ast.Expression.makePlaceholder(body))
  placeholder.isPlaceholder = true

  return placeholder
}

AstRule.parse = function (source) {
  return Parser.parse(source, {startRule: 'RuleOnly'})
}

AstRule.prototype.clone = function () {
  const head = this.head.clone()
  const body = this.body.clone()
  const rule = new AstRule(head, body, Ast.makeLocation(this.location))

  if (this.isPlaceholder) {
    rule.isPlaceholder = true
  }

  rule.default = this.default
  rule.else = this.else != null ? this.else.clone() : null

  return rule
}

/** @returns {string} The syntax name of the rule (e.g., `else`). */
AstRule.prototype.getName = function () {
  return `${this.head.name}`
}

/** @returns {string} The name of the root rule in an else chain. */
AstRule.prototype.getRootName = function () {
  return this.isElse() ? this.root.getName() : this.getName()
}

AstRule.prototype.isComplete = function () {
  return this.head.getKind() === Ast.RuleHead.Kind.COMPLETE
}

AstRule.prototype.isPartialObject = function () {
  return this.head.getKind() === Ast.RuleHead.Kind.PARTIAL_OBJECT
}

AstRule.prototype.isPartialSet = function () {
  return this.head.getKind() === Ast.RuleHead.Kind.PARTIAL_SET
}

AstRule.prototype.isConstant = function () {
  return this.body.value.every((x) => x.isTrue())
}

AstRule.prototype.isFunction = function () {
  return this.head.hasArguments()
}

AstRule.prototype.hasElse = function () {
  return this.else != null
}

AstRule.prototype.isElse = function () {
  return this.root != null
}

AstRule.prototype.setDefault = function (boolean) {
  this.default = boolean
  return this
}

/**
 * Adds `elseRule` as this rule’s `else` property (replacing any subsequent
 * `else` rules if an existing chain continued from here).
 */
AstRule.prototype.setElse = function (elseRule) {
  if (elseRule.getName() !== 'else') {
    const location = Ast.makeLocation(elseRule.head.name.location)
    elseRule.head.name = new Ast.Variable('else', location)
  }

  elseRule.root = this.isElse() ? this.root : this
  delete elseRule.arguments

  this.else = elseRule
  return this
}

/**
 * Removes this rule’s `else` property (and any subsequent `else` rules if the
 * chain continues from here).
 */
AstRule.prototype.unsetElse = function () {
  delete this.else.root
  delete this.else

  return this
}

AstRule.prototype.toJSON = function () {
  const {head, body, location} = this
  return {
    default: this.default || undefined,
    head,
    body,
    else: this.else || undefined,
    location
  }
}

AstRule.prototype.toString = function () {
  const {RuleHead: {Kind}} = Ast
  const kind = this.head.getKind()
  const head =
    kind === Kind.COMPLETE ? `is a complete value (${this.head})` :
      kind === Kind.PARTIAL_OBJECT ? `defines a key in an object (${this.head})` :
        kind === Kind.PARTIAL_SET ? `defines an element in a set (${this.head})` :
          'is an unknown kind of rule'

  return `${this.getName()} ${head}`
}

export default AstRule
