import * as Ast from '../ast'

function AstRuleHead(name, argumentsOrKey, value, location) {
  this.name = name

  if (argumentsOrKey) {
    if (Array.isArray(argumentsOrKey)) {
      this.arguments = argumentsOrKey
    } else {
      this.key = argumentsOrKey
    }
  }

  const hasOperator = !!value && !!value.operator
  const {operator, term} = hasOperator ? value : {term: value}

  this.assign = !!operator && `${operator}` === Ast.Builtin.get(':=').name
  this.value = term
  this.location = location
}

AstRuleHead.TYPE = 'rulehead'

AstRuleHead.Kind = {
  COMPLETE: 'complete',
  PARTIAL_OBJECT: 'object',
  PARTIAL_SET: 'set'
}

AstRuleHead.prototype.clone = function () {
  const argumentsOrKey = this.arguments || this.key
  return new AstRuleHead(this.name, argumentsOrKey, this.value, this.location)
}

AstRuleHead.prototype.getKind = function () {
  if (this.key) {
    if (this.value) {
      return AstRuleHead.Kind.PARTIAL_OBJECT
    }

    return AstRuleHead.Kind.PARTIAL_SET
  }

  return AstRuleHead.Kind.COMPLETE
}

AstRuleHead.prototype.hasArguments = function () {
  return !!this.arguments
}

AstRuleHead.prototype.toJSON = function () {
  const {arguments: args, assign, name, key, value, location} = this
  return {
    name: String(name),
    arguments: args,
    assign,
    key,
    value,
    location
  }
}

AstRuleHead.prototype.toString = function () {
  const {arguments: args, key, name, value} = this

  const keyString = ((isString) => {
    if (!key) {
      return ''
    }

    return isString ? `["${key}"]` : `[${key}]`
  })(key instanceof Ast.String)

  const input = args
    ? `(${args.map((arg) => arg instanceof Ast.String ? `"${arg}"` : arg).join(', ')})`
    : keyString
  const output = value === undefined ? '' : ` = ${value}`

  return `${name}${input}${output}`
}

export default AstRuleHead
