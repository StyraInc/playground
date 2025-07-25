import capabilities from './capabilities.generated.json'

import {default as AstArray} from './ast/array'
import {default as AstBoolean} from './ast/boolean'
import {default as AstEvery} from './ast/every'
import {default as AstExpression} from './ast/expression'
import {default as AstNull} from './ast/null'
import {default as AstNumber} from './ast/number'
import {default as AstObject} from './ast/object'
import {default as AstReference} from './ast/reference'
import {default as AstRule} from './ast/rule'
import {default as AstSet} from './ast/set'
import {default as AstSome} from './ast/some'
import {default as AstString} from './ast/string'
import {default as AstVariable} from './ast/variable'
import {default as AstWith} from './ast/with'

export {AstArray as Array}
export {default as ArrayComprehension} from './ast/array-comprehension'
export {AstBoolean as Boolean}
export {default as Builtin} from './ast/builtin'
export {default as Comment} from './ast/comment'
export {AstEvery as Every}
export {AstExpression as Expression}
export {default as Import} from './ast/import'
export {default as Module} from './ast/module'
export {AstNull as Null}
export {AstNumber as Number}
export {AstObject as Object}
export {default as ObjectComprehension} from './ast/object-comprehension'
export {default as Package} from './ast/package'
export {AstReference as Reference}
export {AstRule as Rule}
export {default as RuleHead} from './ast/rule-head'
export {default as RuleBody} from './ast/rule-body'
export {AstSet as Set}
export {default as SetComprehension} from './ast/set-comprehension'
export {AstSome as Some}
export {AstString as String}
export {AstVariable as Variable}
export {AstWith as With}

export const IDENTIFIER_REGEX = /^[A-Za-z_][A-Za-z_0-9]*$/
export const KEYWORDS = ['as', 'default', 'else', 'false', 'import', 'not',
  'null', 'package', 'some', 'true', 'with']

export const FUTURE_KEYWORDS = capabilities.future_keywords

export function isExpression(node) {
  return node != null && node.constructor === AstExpression
}

export function isReference(node) {
  return node != null && node.constructor === AstReference
}

export function isFunction(node) {
  return node != null && node.constructor === AstRule && node.isFunction()
}

export function isRule(node) {
  return node != null && node.constructor === AstRule
}

export function isSome(node) {
  return node != null && node.constructor === AstSome
}

export function isString(node) {
  return node != null && node.constructor === AstString
}

export function isVariable(node) {
  return node != null && node.constructor === AstVariable
}

export function isWith(node) {
  return node != null && node.constructor === AstWith
}

// -----------------------------------------------------------------------------

export function findContext(node, callback, seen = new Set()) {
  if (!seen.has(node) && node != null && typeof node === 'object') {
    // Break cyclical references (e.g., `node.root`).
    seen.add(node)

    const entries = Object.entries(node)

    for (let i = 0, n = entries.length; i < n; ++i) {
      const [key, value] = entries[i]
      const matched = callback(node, key, value)

      if (matched != null) {
        // Ignore `matched` when it is `false` (c.f., `null` or `undefined`).
        if (matched) {
          return {key, node, value}
        }

        const context = findContext(value, callback, seen)

        if (context) {
          return context
        }
      }
    }
  }
}

// -----------------------------------------------------------------------------

export function fromJsonSchema(schema, value, isValueJson = false) {
  const cast = arguments.length > 1
  const defaultValue = value ?? schema.default

  switch (schema.type) {
    case 'array': {
      const {items, uniqueItems} = schema

      if (uniqueItems) {
        return !defaultValue
          ? new AstSet()
          : new AstSet(
            (isValueJson ? defaultValue : [...defaultValue])  // json uses array as set
              .map((x) => fromJsonSchema(items, x, isValueJson))
          )
      }

      return !defaultValue
        ? new AstArray()
        : new AstArray(defaultValue.map((x) => fromJsonSchema(items, x, isValueJson)))
    }

    case 'boolean': return cast
      ? new AstBoolean(Boolean(defaultValue))
      : new AstBoolean()

    case 'null': return new AstNull()

    case 'number': return cast && !isNaN(defaultValue)
      ? new AstNumber(Number(defaultValue))
      : new AstNumber()

    case 'object': {
      const {additionalProperties, properties} = schema

      if (properties) {
        const {['hint:order']: order} = schema
        const entries = Object.entries(properties)

        if (order) {
          entries.sort(([a], [b]) => {
            const ax = order.indexOf(a)
            const bx = order.indexOf(b)

            return ax > -1 ? (ax < bx ? -1 : 1) : 1
          })
        }

        const pairs = entries.map(([k, v]) => {
          const key = new AstString(k)
          return [key, fromJsonSchema(v, defaultValue && defaultValue[k], isValueJson)]
        })

        return new AstObject(pairs)
      }

      if (additionalProperties && additionalProperties.constructor === Object) {
        const keys = Object.keys(defaultValue ? defaultValue : [])

        const pairs = keys.map((k) => {
          const key = new AstString(k)
          return [key, fromJsonSchema(additionalProperties, defaultValue && defaultValue[k], isValueJson)]
        })

        return new AstObject(pairs)
      }

      return new AstObject()
    }

    case 'string': return cast && defaultValue
      ? new AstString(String(defaultValue))
      : new AstString()

    default: {
      const {enum: list} = schema

      if (list && list.length) {
        return fromNative(list[0])
      }

      throw new Error(`Unexpected type in \`schema\`: ${JSON.stringify(schema)}`)
    }
  }
}

// -----------------------------------------------------------------------------

const AstTypesByPrimitiveType = {
  [Boolean]: AstBoolean,
  [Number]: AstNumber,
  [String]: AstString,
  [AstExpression]: AstExpression
}

export function fromNative(native) {
  if (native === null) {
    return new AstNull()
  }

  const {constructor} = native
  const AstType = AstTypesByPrimitiveType[constructor]

  if (AstType) {
    return new AstType(native)
  }

  if (constructor === Array) {
    return new AstArray(native.map((x) => fromNative(x)))
  }

  if (constructor === Object) {
    const keys = Object.keys(native)
    return new AstObject(keys.map((x) => [fromNative(x), fromNative(native[x])]))
  }

  if (constructor === Set) {
    const converted = []
    native.forEach((x) => converted.push(fromNative(x)))
    return new AstSet(converted)
  }

  throw new Error(`Unxpected value: ${native}`)
}

// -----------------------------------------------------------------------------

/**
 * @returns {Object} A clone of `location` if it’s defined; otherwise, a
 *     synthetic default location.
 */
export function makeLocation(location) {
  if (location) {
    return {end: {...location.end}, start: {...location.start}}
  }

  return {
    end: {
      column: 1,
      line: 1,
      offset: 0
    },
    start: {
      column: 1,
      line: 1,
      offset: 0
    }
  }
}

/**
 * @returns {Object} A CodeMirror 0-indexed version of `location` if it’s
 *     defined; otherwise, a synthetic default location.
 */
export function makeCodeMirrorLocation(location) {
  if (location) {
    const {end, start} = location

    return {
      end: {
        ch: end.column - 1,
        line: end.line - 1
      },
      start: {
        ch: start.column - 1,
        line: start.line - 1
      }
    }
  }

  return {
    end: {
      ch: 0,
      line: 0
    },
    start: {
      column: 0,
      line: 0
    }
  }
}
