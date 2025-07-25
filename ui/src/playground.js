///////////////////////////////////////////////////////////////////////////////
// Link the css file for webpack
import 'bulma/css/bulma.min.css'
import 'bulma-tooltip/dist/css/bulma-tooltip.min.css'

// Import ours last! It gets precedence on style..
import './playground.css'

///////////////////////////////////////////////////////////////////////////////

import Split from 'split.js'
import debounce from 'lodash.debounce'
import policyTypes from './policies'
import JSONBigInt from 'json-bigint'
import { CodeMirrorWrapper } from './CodeMirrorWrapper.js'

/// notification toast
import { Notyf } from 'notyf'
import 'notyf/notyf.min.css'
const notyf = new Notyf({ripple: false})

///////////////////////////////////////////////////////////////////////////////

import { getGist, decodeKey} from './gists'

// Constants

const orderedPolicyTypes = ['all', 'access-control', 'ai', 'envoy', 'kubernetes']

const SAMPLE_MODULE = `package play

import rego.v1

# Welcome to the Rego playground! Rego (pronounced "ray-go") is OPA's policy language.
#
# Try it out:
#
#   1. Click Evaluate. Note: 'hello' is 'true'
#   2. Change "world" to "hello" in the INPUT panel. Click Evaluate. Note: 'hello' is 'false'
#   3. Change "world" to "hello" on line 25 in the editor. Click Evaluate. Note: 'hello' is 'true'
#
# Features:
#
#         Examples  browse a collection of example policies
#         Coverage  view the policy statements that were executed
#         Evaluate  execute the policy with INPUT and DATA
#          Publish  share your playground and experiment with local deployment
#            INPUT  edit the JSON value your policy sees under the 'input' global variable
#    (resize) DATA  edit the JSON value your policy sees under the 'data' global variable
#           OUTPUT  view the result of policy execution

default hello := false

hello if input.message == "world"
`

const SAMPLE_INPUT = `{
    "message": "world"
}`

const MODULE_NAME = 'policy.rego'

// Needed for OPA run and query
// const BASE_URL = window.location.origin

const PlaygroundModes = {
  SHARE: 'SHARE',
  PUBLISH: 'PUBLISH',
  NEW: 'NEW'
}

const Api = {
  DATA: '/v1/data',
  PUBLISH_V2: '/v2/publish',
  [PlaygroundModes.PUBLISH]: '/v1/distribute',
  [PlaygroundModes.SHARE]: '/v1/share',
  VARS: '/v1/vars',
  FORMAT: '/v1/fmt',
  LINT: '/v1/lint',
  AUTH: '/v2/auth',
  TESTAUTH: '/v2/auth/test'
}

const PlaygroundPanes = {
  INPUT: 'input',
  DATA: 'data',
  OUTPUT: 'output',
  LINT: 'lint',
}

const RegexKeys = {
  [PlaygroundModes.SHARE]: /\w+:\/\/.*\/(p|play)\/([A-Za-z0-9_-]+\/*)/,
  [PlaygroundModes.PUBLISH]: /\w+:\/\/.*\/(d|distribute)\/([A-Za-z0-9_-]+\/*)/
}

///////////////////////////////////////////////////////////////////////////////
// Globals

// playgroundState is used by many of the application functions to track
// references to various objects, elements, and save state of the
// current playground instance.
// The values set below are to be considered defaults.
const playgroundState = {
  mode: PlaygroundModes.NEW,
  modes: {
    [PlaygroundModes.NEW]: {
      policy: SAMPLE_MODULE,
      input: SAMPLE_INPUT,
      data: null
    },

    [PlaygroundModes.PUBLISH]: {
      policy: null,
      input: null,
      data: null,
      url: null,
      regoVersionV1Mode: false
    },

    [PlaygroundModes.SHARE]: {
      policy: null,
      input: null,
      data: null,
      url: null,
      gistURL: null,
      output: null,
      coverage: false,
      strict: false,
      strictBuiltInErrors: false,
      allBuiltInErrors: false,
      regoVersionV1Mode: false
    },
  },

  // CodeMirror instances
  editors: {
    data: null,
    input: null,
    output: null,
    policy: null
  },

  statusWidgets: {
    data: null,
    input: null,
    output: null,
    policy: null
  },

  traceValuesEditor: null,

  strictMode: true,

  // Rego version
  regoVersionV1Mode: true,
  dismissedV0Warning: false,

  // Coverage
  showCoverage: false,
  previousCoverage: undefined,

  // Built-in errors
  strictBuiltInErrors: false,
  allBuiltInErrors: false,

  // Live linting settings
  linting: {
    liveLinting: {
      // the live linting feature is enabled by default
      enabled: true,
      // the maximum time a linting request can take before live-linting is disabled to maintain performance
      budget: 1500,
      // the time to wait after editing has stopped before running the linter
      delay: 1000,
      // the timeout function for the delayed linting run after an editing change,
      // this is set using setTimeout when the editor changes
      timeout: null,
    },
    // the timestamp of the last linting run
    lastRun: 0,
  },

  panes: {
    [PlaygroundPanes.INPUT]: {
      size: 50,
      element: document.querySelector('#input-pane')
    },
    [PlaygroundPanes.DATA]: {
      size: 0,
      element: document.querySelector('#data-pane')
    },
    [PlaygroundPanes.OUTPUT]: {
      size: 50,
      element: document.querySelector('#output-pane')
    },
  },

  // split.js instances
  sidePanelSplit: null,
  ioSplit: null,

  dropdownWidth: null,

  tooltipText: {},

  // if true, clicking Evaluate will log a pre-populating ?state=... link to the console.
  // logStateLink must be set manually by those who want to use the feature.
  logStateLink: false,
}

///////////////////////////////////////////////////////////////////////////////

function loadRegoVersionInfo(){
  const regoVersion = document.querySelector('.rego-version')
  if (playgroundState.regoVersionV1Mode) {
    regoVersion.textContent = 'Rego (v1)'
  } else {
    regoVersion.textContent = 'Rego (v0)'
  }
}

function loadVersionInfo() {
  const platform = window.navigator.platform
  const macRegex = /^mac/i
  const winRegex = /^win/i

  fetch('/version')
    .then((response) => response.json())
    .then(({ opa_version, opa_release_version, regal_version }) => {
      for (let opaVersion of document.querySelectorAll('.opa-version')) {
        opaVersion.textContent = 'OPA ' + opa_version
        if (regal_version !== '') {
          const regalFrag = document.createDocumentFragment()
          const link = document.createElement('a')
          link.setAttribute('href', 'https://docs.styra.com/regal')
          link.setAttribute('target', '_blank')
          link.innerHTML = 'Regal'
          regalFrag.appendChild(document.createTextNode(', '))
          regalFrag.appendChild(link)
          regalFrag.appendChild(document.createTextNode(' ' + regal_version))
          opaVersion.appendChild(regalFrag)
        }
      }
      for (let opaVersion of document.querySelectorAll('.opa-release-version')) {
        opaVersion.textContent = opa_release_version
      }

      const binaries = {
        linux: {
          tab: document.querySelector('#linux'),
          install: `curl -L -o opa \\\nhttps://openpolicyagent.org/downloads/${opa_release_version}/opa_linux_amd64`
        },
        mac: {
          tab: document.querySelector('#mac'),
          install: `curl -L -o opa \\\nhttps://openpolicyagent.org/downloads/${opa_release_version}/opa_darwin_amd64`
        },
        win: {
          tab: document.querySelector('#win'),
          install: `https://openpolicyagent.org/downloads/${opa_release_version}/opa_windows_amd64.exe`
        }
      }

      const onClick = (tab, install, os) => {
        Object.values(binaries).forEach((b) => {
          b.tab.parentElement.classList.remove('is-active')
        })
        tab.parentElement.classList.add('is-active')

        osText.textContent = `${install}; \\\nchmod 755 ./opa`

        const copyButton = document.querySelector('.copy-os-button')
        if (os === 'win') {
          osText.innerHTML = `\n<a href="${install}" download>Download OPA (Windows 64-bit)</a>\n `
          copyButton.style.display = 'none'
        } else {
          copyButton.style.display = 'initial'
        }
      }

      const osText = document.querySelector('.os-text')
      Object.entries(binaries).forEach(([os, { tab, install }]) => {
        tab.addEventListener('click', () => {
          onClick(tab, install, os)
        })
      })

      const currentOS = macRegex.test(platform) ? 'mac' : winRegex.test(platform)
        ? 'win' : 'linux'
      onClick(binaries[currentOS].tab, binaries[currentOS].install, currentOS)
    })
    .catch((error) => {
      console.log(error)
    })
}

function onDocChanged() {
  handleEditorChange()
  if (!playgroundState.linting.liveLinting.enabled) {
    return
  }
  handleLint()
}

function handleEditorChange() {

  // disable instruction button
  if (document.cookie.indexOf('experimental') > -1 ) {
    document.querySelector('#instructions-button').style.display = 'none'
    document.querySelector('#publish-button').removeAttribute('disabled')
    document.querySelector('#publish-button').style.display = 'flex'
    return
  }

  const { mode } = playgroundState

  const isChanged = mode !== PlaygroundModes.NEW || hasChanges()
  const publishButton = document.querySelector('#publish-button')
  const instructionsButton = document.querySelector('#instructions-button')

  if (isChanged) {
    publishButton.removeAttribute('disabled')
    instructionsButton.style.display = 'none'
    publishButton.style.display = 'flex'
  } else {
    publishButton.setAttribute('disabled', true)
  }
}

function initCodeMirror(parent, options) {
  const { value, lang, evalButtonText, onLineNumberClick, onModEnter, formatHandler, readOnly } = options
  return new CodeMirrorWrapper({
    parent,
    lang,
    evalButtonText,
    onLineNumberClick,
    onModEnter,
    initialValue: value,
    onDocChanged,
    formatHandler,
    readOnly,
  })
}

function handleLint() {
  playgroundState.linting.lastRun = Date.now()
  playgroundState.editors.policy.clearDiagnostics()

  const startTime = performance.now()
  fetch(Api.LINT, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSONBigInt.stringify({
      rego_version: playgroundState.regoVersionV1Mode ? 1 : 0,
      rego_module: playgroundState.editors.policy.getValue()
    })
  }).then(function (resp) {
    return resp.json().then(function (result) {
      if (result.error_message === '') {
        // this is the success case where there are no errors and 0 or more violations
        if (result.report.summary.num_violations === 0) {
          // no violations so we can exit early
          return
        }

        playgroundState.editors.policy.setDiagnostics(result.report.violations)
      } else if (result.errors.length > 0) {
        // this is the main error case where there are Rego parser errors that prevent linting
        playgroundState.editors.policy.setDiagnostics(result.errors)
      }
    })
  }).catch(function (error) {
    console.log(error)
  }).finally(function () {
    const lintTime = performance.now() - startTime
    if (lintTime > playgroundState.linting.liveLinting.budget && playgroundState.linting.liveLinting.enabled) {
      handleLintMode()
      notyf.error('Live Linting has been disabled to maintain performance. Click <i>Evaluate</i> to run the linter.')
    }
  })
}

function initEditors() {
  const { mode, modes } = playgroundState

  const opts = {
    lang: 'rego',
    value: versionRegoSyntax(modes[mode].policy),
    evalButtonText: document.querySelector('#evaluate-button'),
    onLineNumberClick: (line) => {
      document.location.hash = 'L' + line
      playgroundState.editors.policy.highlightLine(line)
    },
    onModEnter: handleEvaluate,
    formatHandler: handleFormat,
  }

  const policyEditor = document.querySelector('#policy-editor')
  playgroundState.editors.policy = initCodeMirror(policyEditor, opts)

  const line = getLineFromHash()
  if (line) {
    playgroundState.editors.policy.highlightLine(line)
  }

  const panes = playgroundState.panes
  const inputPane = panes[PlaygroundPanes.INPUT].element
  const dataPane = panes[PlaygroundPanes.DATA].element
  const outputPane = panes[PlaygroundPanes.OUTPUT].element

  playgroundState.editors.input =
    initCodeMirror( inputPane.querySelector('.editor'), {
      value: modes[mode].input
    })

  const data = modes[mode].data
  if (data) {
    Object.values(playgroundState.panes).forEach((pane) => {
      pane.size = 33
    })
  }

  playgroundState.editors.data =
    initCodeMirror( dataPane.querySelector('.editor'), {
      value: data || '{\n    \n}'
    })

  playgroundState.editors.output =
    initCodeMirror( outputPane.querySelector('.editor'), {
      readOnly: true
    })
}

// Idempotent layout updater, call at least once any time the
// playground state has changed. (e.g. button click or API callback with data).
function updateLayout() {
  // Always split the main editor and side-panel.
  if (playgroundState.sidePanelSplit == null) {
    playgroundState.sidePanelSplit = Split(['#policy-editor', '#side-panel'], {
      sizes: [60, 40],
      gutterSize: 4,
    })
  }

  const splitTargets = []
  const sizes = []

  Object.values(playgroundState.panes).forEach(({ size, element }) => {
    sizes.push(size)
    splitTargets.push(element)
  })

  // Start with a clean side panel
  if (playgroundState.ioSplit !== null) {
    playgroundState.ioSplit.destroy({ preserveStyles: false, preserveGutters: true })
    playgroundState.ioSplit = null
  }

  const panelTitle = document.querySelector('.panel-title')
  playgroundState.ioSplit = Split(splitTargets, {
    sizes,
    snapOffset: 0,
    direction: 'vertical',
    gutterSize: 4,
    minSize: panelTitle.offsetHeight // Height of the title bar for each panel
  })

  if (playgroundState.mode === PlaygroundModes.SHARE) {
    const state = playgroundState.modes[PlaygroundModes.SHARE]

    toggle('strict-switch', state.strict)
    toggle('explain-switch', state.coverage)

    if (state.output !== null) {
      displayOutput(state.output, 'output')
      displayCoverage()
    }
  }
}

function toggle(id, value) {
  const elm = document.getElementById(id)
  if (value !== elm.checked) {
    elm.click()
  }
}

// formatNanosecondTime takes a integer representing a nanosecond duration and
// returns a string representing the duration in a human-readable format.
function formatNanosecondTime(nanoseconds) {
  if (nanoseconds < 1000) {
    return `${nanoseconds}ns`
  } else if (nanoseconds < 1000000) {
    return `${Math.round(nanoseconds / 1000)}µs`
  } else if (nanoseconds < 1000000000) {
    return `${Math.round(nanoseconds / 1000000)}ms`
  } else {
    return `${Math.round(nanoseconds / 1000000000)}s`
  }
}

// Returns an object with `comment` and `message` properties
// for the output of an evaluation.
function formatOutput(value) {

  const results = value.result

  let consoleHint = formatConsoleHint(value)

  if (results == null || !results.length) {
    return { comment: `No results found in ${formatNanosecondTime(value.eval_time)}.${consoleHint}`, message: '' }
  }

  if (results[0].bindings) {
    return formatBindingOutput(results, formatNanosecondTime(value.eval_time), consoleHint)
  }

  return formatExpressionOutput(results, formatNanosecondTime(value.eval_time), consoleHint)
}

function formatConsoleHint(value) {
  let consoleHint = ''

  if ('output' in value && value.output.length > 0) {
    consoleHint = ' See print() output in console.'
  }

  if ('ignored' in value && value.ignored.length > 0) {
    consoleHint += ` ${value.ignored.length} statement(s) from the selection were ignored. See output in console for more info.`
  }

  return consoleHint
}

function formatExpressionOutput(results, time, consoleHint) {
  let value = undefined

  // If there is a single value that can be displayed, just output it. If not,
  // then output the set of expression values by turning the set of sets into
  // a set of objects where the keys are expression texts and the values are
  // the expression values. Deduplicate identical expressions.
  if (results.length === 1 && results[0].expressions.length === 1) {
    value = results[0].expressions[0].value
  } else {
    value = results.map((result) => {
      let seen = {}
      return result.expressions.reduce((obj, expr) => {
        let text = expr.text
        if (expr.text in seen) {
          text = expr.text + ' # (' + seen[expr.text] + ')'
          seen[expr.text] += 1
        } else {
          seen[expr.text] = 1
        }
        obj[text] = expr.value
        return obj
      }, {})
    })
  }

  const nResults = results.length
  const nResultsString = nResults === 1 ? 'result' : 'results'
  const comment = `Found ${nResults} ${nResultsString} in ${time}.${consoleHint}`

  return { comment: comment, message: JSONBigInt.stringify(value, null, 4) }
}

function formatBindingOutput(results, time, consoleHint) {
  const nResults = results.length
  const nResultsString = nResults === 1 ? 'result' : 'results'
  const comment = `Found ${nResults} ${nResultsString} in ${time}.${consoleHint}`
  const resultSet = results.map((result) => {
    return result.bindings
  })
  return { comment: comment, message: JSONBigInt.stringify(resultSet, null, 4) }
}

function buildPlaygroundDataRequest() {
  const request = {
    'rego_modules': {
      [MODULE_NAME]: playgroundState.editors.policy.getValue(),
    }
  }

  let output = {
    message: null,
    mode: 'application/json'
  }
  const inputValue = playgroundState.editors.input.getValue()
  try {
    output.message = inputValue
    if (inputValue === '') {
      request.input = ''
      output.status = 'Input was blank. Document will be undefined.'
    } else {
      request.input = JSONBigInt.parse(inputValue)
    }
    displayOutput(output, 'input')
  } catch (error) {
    throw new Error('Failed to parse input: ' + error)
  }

  // Reset the output object for the next pane.
  output = {
    message: null,
    mode: 'application/json'
  }
  const dataValue = playgroundState.editors.data.getValue()
  try {
    output.message = dataValue
    if (dataValue === '') {
      request.data = ''
      output.status = 'Data was blank. Document will be undefined.'
    } else {
      request.data = JSONBigInt.parse(dataValue)
    }
    displayOutput(output, 'data')
  } catch (error) {
    throw new Error('Failed to parse data: ' + error)
  }

  const query = playgroundState.editors.policy.getSelection()
  if (query) {
    request.rego = query
  }

  return request
}

function displayOutput(output, target) {
  const { editors } = playgroundState

  // For plain text mode enable line wrapping
  const shouldWrap = (output.mode === 'text/plain' || output.mode === '')
  editors[target].setLineWrapping(shouldWrap)

  editors[target].setValue(output.message)

  // Add a line widget for the result comment
  if (playgroundState.statusWidgets[target] != null) {
    playgroundState.statusWidgets[target].clear()
    playgroundState.statusWidgets[target] = null
  }

  // Only show the status widget if there is a status
  if (output.status) {
    const container = document.createElement('div')
    container.innerText = output.status
    container.classList.add('eval-comment')
    playgroundState.editors.output.setStatusBar(output.status)
  }
}

function removeLineHighlights(cm) {
  cm.clearHighlightedLine()
}

function displayCoverage() {
  if (playgroundState.currentCoverage === undefined) {
    return
  }

  const covered = playgroundState.currentCoverage.files['policy.rego'].covered || []

  let lines = []
  for (let i = 0; i < covered.length; i++) {
    for (let line = covered[i].start.row; line <= covered[i].end.row; line++) {
      lines.push(line)
    }
  }
  playgroundState.editors.policy.setCoverage(lines)
}

function hideCoverage() {
  playgroundState.editors.policy.resetCoverage()
}

function handleEvaluate() {
  const evalButton = document.querySelector('#evaluate-button')
  evalButton.classList.add('is-loading')

  if (!playgroundState.linting.liveLinting.enabled) {
    handleLint()
  }

  if (playgroundState.logStateLink === true) {
    logStateLink()
  }

  let data
  try {
    data = buildPlaygroundDataRequest()
  } catch (error) {
    displayOutput({
      status: 'Unable to send request.',
      message: '' + error,
      mode: 'text/plain',
    }, 'output')
    evalButton.classList.remove('is-loading')
    return
  }

  let output = { status: '', message: '', mode: '' }

  let requestURL = Api.DATA

  if (playgroundState.strictMode) {
    data['strict'] = true
  }

  data['rego_version'] = playgroundState.regoVersionV1Mode ? 1 : 0

  if (playgroundState.strictBuiltInErrors) {
    data['built_in_errors_strict'] = true
  }

  if (playgroundState.allBuiltInErrors) {
    data['built_in_errors_all'] = true
  }

  if (playgroundState.showCoverage) {
    // unset the current coverage before making the request. If the response
    // is successful, the current coverage will be reset. If anything goes
    // wrong, the coverage will be removed.
    playgroundState.currentCoverage = undefined
    data['coverage'] = true
  }

  fetch(requestURL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSONBigInt.stringify(data)
  }).then(function (resp) {
    return resp.json().then(function (result) {
      // With the response body as parsed JSON..

      if (resp.status === 200) {
        if ('output' in result && result.output.length > 0) {
          console.log(result.output)
        }
        let formatted = formatOutput(result)
        output = {
          status: formatted.comment,
          message: formatted.message,
          mode: 'rego',
          trace: result.trace,
        }
        playgroundState.currentCoverage = result.coverage

        if (result.rego_version != data['rego_version'] && result.rego_version == 0) {
          handleRegoVersionV0Mode(false)
          const warningElem = document.getElementById('v0-upgrade-message')
          if (warningElem) {
            warningElem.classList.remove('display-none')
          }
        }
      } else {
        let status = `Error Evaluating.${formatConsoleHint(result)}`
        output = {
          status: status,
          message: result.message,
          mode: 'text/plain',
          trace: result.trace,
        }
      }

      if ('ignored' in result && result.ignored.length > 0) {
        console.log(result.ignored.join('\n'))
      }
    })
  }).catch(function (error) {
    console.log(error)

    output = {
      status: 'Unexpected Error',
      message: `${error}`,
      mode: '',
    }
  }).finally(function () {
    displayOutput(output, 'output')
    displayCoverage()
    evalButton.classList.remove('is-loading')
  })
}

function handleStrictMode() {
  playgroundState.strictMode = !playgroundState.strictMode
}

function handleRegoVersionV0Mode(updatePolicy = true) {
  playgroundState.regoVersionV1Mode = false
  updateOptionsFromState(playgroundState)
  loadRegoVersionInfo()

  if (updatePolicy) {
    const currentText = playgroundState.editors.policy.getValue()
    playgroundState.editors.policy.setValue(versionRegoSyntax(currentText))
  }

  // run linter as rules are different depending on regoVersion
  handleLint()
}

function handleRegoVersionV1Mode(updatePolicy = true) {
  playgroundState.regoVersionV1Mode = !playgroundState.regoVersionV1Mode
  updateOptionsFromState(playgroundState)
  loadRegoVersionInfo()

  const warningElem = document.getElementById('v0-upgrade-message')
  if (warningElem) {
    warningElem.classList.add('display-none')
  }

  if (updatePolicy) {
    const currentText = playgroundState.editors.policy.getValue()
    playgroundState.editors.policy.setValue(versionRegoSyntax(currentText))
  }

  // run linter as rules are different depending on regoVersion
  handleLint()
}

function versionRegoSyntax(text) {
  if (playgroundState.regoVersionV1Mode) {
    return text.replace(/^import rego\.v1\n+/m, '')
  } else {
    if (!/^import rego\.v1\n/m.test(text)) {
      let lines = text.split('\n')
      let packageLineIndex = lines.findIndex(line => /^package/.test(line))
      if (packageLineIndex !== -1) {
        lines.splice(packageLineIndex + 1, 0, '\nimport rego.v1')
      }
      return lines.join('\n')
    }
  }

  return text
}

function handleCoverage() {
  playgroundState.showCoverage = !playgroundState.showCoverage
  if (!playgroundState.showCoverage) {
    hideCoverage()
  } else {
    if (playgroundState.currentCoverage !== undefined) {
      displayCoverage()
    }
  }
}

function handleStrictBuiltInErrors() {
  playgroundState.strictBuiltInErrors = !playgroundState.strictBuiltInErrors
  if (playgroundState.allBuiltInErrors) {
    playgroundState.allBuiltInErrors = false
  }
  updateOptionsFromState(playgroundState)
}

function handleAllBuiltInErrors() {
  playgroundState.allBuiltInErrors = !playgroundState.allBuiltInErrors
  if (playgroundState.strictBuiltInErrors) {
    playgroundState.strictBuiltInErrors = false
  }
  updateOptionsFromState(playgroundState)
}

function handleDefaultBuiltInErrors() {
  playgroundState.strictBuiltInErrors = false
  playgroundState.allBuiltInErrors = false
  updateOptionsFromState(playgroundState)
}

function handleLintMode() {
  playgroundState.linting.liveLinting.enabled = !playgroundState.linting.liveLinting.enabled

  if (playgroundState.linting.liveLinting.enabled) {
    handleLint()
  }

  updateOptionsFromState(playgroundState)
}

function updateOptionsFromState(state = playgroundState) {
  const strictSwitch = document.querySelector('#strict-switch')
  const coverageSwitch = document.querySelector('#explain-switch')
  const regoVersionV0Switch = document.querySelector('#rego-version-v0-switch')
  const regoVersionV1Switch = document.querySelector('#rego-version-v1-switch')
  const strictBuiltInsSwitch = document.querySelector('#strict-built-in-errors-switch')
  const allBuiltInErrorsSwitch = document.querySelector('#all-built-in-errors-switch')
  const defaultBuiltInErrorsSwitch = document.querySelector('#default-built-in-errors-switch')
  const liveLinting = document.querySelector('#live-linting-switch')

  strictSwitch.checked = state.strictMode
  coverageSwitch.checked = state.showCoverage
  regoVersionV0Switch.checked = !state.regoVersionV1Mode
  regoVersionV1Switch.checked = state.regoVersionV1Mode
  strictBuiltInsSwitch.checked = state.strictBuiltInErrors
  allBuiltInErrorsSwitch.checked = state.allBuiltInErrors
  defaultBuiltInErrorsSwitch.checked = !state.allBuiltInErrors && !state.strictBuiltInErrors
  liveLinting.checked = state.linting.liveLinting.enabled
}

function hasChanges(mode = playgroundState.mode) {
  const input = playgroundState.editors.input.getValue()
  const data = playgroundState.editors.data.getValue()
  const policy = playgroundState.editors.policy.getValue()
  const state = playgroundState.modes[mode]

  if (mode === PlaygroundModes.PUBLISH || mode === PlaygroundModes.SHARE) {
    const regoVersionV1Mode = playgroundState.regoVersionV1Mode
    return state.policy !== policy || state.input !== input || state.data !== data || state.regoVersionV1Mode !== regoVersionV1Mode
  }
  return state.policy !== policy || state.input !== input || state.data !== data
}

function copy(value) {
  const node = document.createElement('textarea')
  node.value = value
  document.body.appendChild(node)

  const style = node.style
  style.position = 'fixed'
  style.left = '-99999em'
  style.top = 0

  node.select()
  const okay = document.execCommand('copy')

  document.body.removeChild(node)

  if (!okay) {
    throw new Error('Failed to copy string')
  }
}

function showModal(url, gisturl, changed, status) {
  document.querySelectorAll('.has-not-modal-tooltip').forEach((node) => {
    node.classList.remove('has-tooltip-bottom')
    playgroundState.tooltipText[node.id] = node.getAttribute('data-tooltip')
    node.removeAttribute('data-tooltip')
  })

  const modal = document.querySelector('.modal')
  modal.classList.add('is-active')

  const statusTag = modal.querySelector('.status-tag')
  statusTag.textContent = status
  if (!changed) {
    statusTag.classList.remove('is-success')
  } else {
    statusTag.classList.add('is-success')
  }

  const urlText = modal.querySelector('.new-share-url')
  urlText.textContent = url

  const gistURLText = modal.querySelector('.new-gist-url')
  const linkElement = document.createElement('a')
  linkElement.href = gisturl
  linkElement.textContent = gisturl
  linkElement.target = '_blank'
  gistURLText.textContent=''
  gistURLText.appendChild(linkElement)

  // TODO enable bundles
  document.querySelector('#run-opa').style.display =  'none'
  document.querySelector('#query-opa').style.display =  'none'
  //   const runText = modal.querySelector('.run-text')
  //   const resource = `bundles/${id}`
  //   runText.textContent = `./opa run --server \\
  // --set decision_logs.console=true \\
  // ${BASE_URL}/${resource}`

//   const queryText = modal.querySelector('.query-text')
//   queryText.textContent = `curl ${BASE_URL}/v1/input/${id} \\\n| curl localhost:8181/v1/data -d @-`
}

function closeModal() {
  const modal = document.querySelector('.modal')
  modal.classList.remove('is-active')

  document.querySelectorAll('.has-not-modal-tooltip').forEach((node) => {
    node.classList.add('has-tooltip-bottom')
    node.setAttribute('data-tooltip', playgroundState.tooltipText[node.id])
  })
}

function handleInstructions(event) {
  const { PUBLISH, SHARE } = PlaygroundModes
  const {
    [SHARE]: shareState
  } = playgroundState.modes

  const modal = document.querySelector('.modal')
  modal.querySelector('.denied-level').style.display =  'none'
  modal.querySelector('.share-level').style.display =  'none'
  modal.querySelector('#under-construction').style.display =  'none'

  if (!hasChanges(PUBLISH)) {
    let gisturl = ''
    try {
      let decoded = decodeKey(getlastPathSegment(shareState.url))
      gisturl = 'https://gist.github.com/'+decoded.id+'/'+decoded.revision
    } catch(error){
      console.log('Failed to decode url: ' + error)
    }
    showModal(shareState.url, gisturl, false, 'No changes')
    return
  }

  const button = event.target

  button.classList.add('is-loading')
  fetchPlayground()
    .then(() => {
      let gisturl = ''
      try {
        let decoded = decodeKey(getlastPathSegment(shareState.url))
        gisturl = 'https://gist.github.com/'+decoded.id+'/'+decoded.revision
      } catch(error){
        console.log('Failed to decode url: ' + error)
      }
      playgroundState.mode = PUBLISH
      showModal(shareState.url, gisturl, true, 'NEW')
    })
    .catch((error) => {
      displayOutput({
        'status': 'Failed to create install request',
        'message': error.toString(),
        'mode': 'plain/text',
      }, 'output')
      closeModal()
    })
    .finally(() => {
      button.classList.remove('is-loading')
    })
}

function saveStateLocally() {
  const data = {
    rego_version: playgroundState.regoVersionV1Mode ? 1 : 0,
    policy: playgroundState.editors.policy.getValue(),
    input: playgroundState.editors.input.getValue(),
    data: playgroundState.editors.data.getValue(),
  }

  // save the current state, this will be reloaded after redirected
  sessionStorage.setItem('rego_version', data.rego_version)
  sessionStorage.setItem('policy', data.policy)
  sessionStorage.setItem('input', data.input)
  sessionStorage.setItem('data', data.data)

  sessionStorage.setItem('modes', JSON.stringify(playgroundState.modes))

}

function handlePublish() {
  fetch(Api.TESTAUTH)
  .then((response) => {
    // if token is invalid, redirect user to github auth
    if (response.status !== 200) {
      window.removeEventListener('beforeunload', warnChanges)

      try {
        saveStateLocally()
      } catch (error) {
        displayOutput({
          status: 'Unable to save Playground state.',
          message: '' + error,
          mode: 'text/plain',
        }, 'output')
        return
      }

      window.location.href = new URL(Api.AUTH, window.location.href)
    } else {
      document.querySelector('.modal').querySelector('.denied-level').style.display =  'none'
      handleShowPublishedGist()
    }
  })
}
const getlastPathSegment = (url) => {
  const urlObject = new URL(url) // Create a URL object for robust parsing
  let pathname = urlObject.pathname

  // Remove trailing slash if present
  if (pathname.endsWith('/')) {
    pathname = pathname.slice(0, -1)
  }

  // Find the index of the last slash and extract the segment
  const lastSlashIndex = pathname.lastIndexOf('/')
  return pathname.slice(lastSlashIndex + 1)
}

function handleShowPublishedGist() {
  const { PUBLISH, SHARE } = PlaygroundModes
  const {
    [SHARE]: shareState
  } = playgroundState.modes

  showModal('loading....', 'loading...', false, '')

  if (!hasChanges(PUBLISH)) {
    const urlText = document.querySelector('.new-share-url')
    urlText.textContent = shareState.url

    const statusTag = document.querySelector('.status-tag')
    statusTag.textContent = 'No changes'

    const gistText = document.querySelector('.new-gist-url')
    gistText.textContent = shareState.gistURL

    return
  }

  fetchPlaygroundV2()
    .then(async () => {
      try {
        window.history.pushState('', '', shareState.url)
      } catch (error) {
        console.log('Failed to update current URL: ' + error)
      }

      let status = 'UPDATED'
      if (playgroundState.mode !== PUBLISH) {
        status = 'NEW'
        playgroundState.mode = PUBLISH
      }

      try {
        let gistURL = await decodeKey(getlastPathSegment(shareState.url))
        shareState.gistURL = gistURL.url
        showModal(shareState.url, shareState.gistURL, true, status)
      } catch(error){
        console.log('Failed to decode url: ' + error)
      }
    })
    .catch((error) => {
      displayOutput({
        'status': 'Failed to create publish request',
        'message': error.toString(),
        'mode': 'plain/text',
      }, 'output')
      closeModal()
    })

  // Remove highlight if the user had selected a line.
  // This can happen concurrently with the publish operation.
  removeLineHighlights(playgroundState.editors.policy)
}

function fetchPlayground() {
  return new Promise((resolve, reject) => {
    let data
    try {
      data = buildPlaygroundDataRequest()
    } catch (error) {
      reject(error)
    }

    if (playgroundState.regoVersionV1Mode) {
      data['rego_version'] = 1
    }

    const fetchParams = {
      body: JSONBigInt.stringify(data),
      headers: { 'Content-Type': 'application/json' },
      method: 'POST'
    }

    let resolved = 0
    const modes = [PlaygroundModes.SHARE, PlaygroundModes.PUBLISH]
    modes.forEach((mode) => {
      if (mode === PlaygroundModes.NEW) {
        return
      }

      let api = Api[mode]
      let params = fetchParams

      if (mode === PlaygroundModes.PUBLISH) {
        const id = playgroundState.modes[PlaygroundModes.PUBLISH].id
        if (id) {
          api = `${api}/${id}`
          params.method = 'PUT'
        }
      }

      fetch(api, params)
        .then((response) => response.json())
        .then(({ result }) => {
          const state = playgroundState.modes[mode]
          state.url = result
          state.input = playgroundState.editors.input.getValue()
          state.data = playgroundState.editors.data.getValue()
          state.policy = data.rego_modules[MODULE_NAME]
          state.id = result.match(RegexKeys[mode])[2]
          state.regoVersionV1Mode = playgroundState.regoVersionV1Mode

          resolved++
          if (resolved === modes.length) {
            resolve()
          }
        })
        .catch((error) => reject(error)) 
    })
  })
}

function fetchPlaygroundV2() {
  return new Promise((resolve, reject) => {
    let data
    try {
      data = buildPlaygroundDataRequest()
    } catch (error) {
      reject(error)
    }

    if (playgroundState.regoVersionV1Mode) {
      data['rego_version'] = 1
    }

    const fetchParams = {
      body: JSONBigInt.stringify(data),
      headers: { 'Content-Type': 'application/json' },
      method: 'POST'
    }


    let api = Api.PUBLISH_V2
    let params = fetchParams

    const id = playgroundState.modes[PlaygroundModes.SHARE].id
    if (id) {
      api = `${api}/${id}`
      params.method = 'PUT'
    }

    fetch(api, params)
      .then((response) => response.json())
      .then(({ bundle_id, bundle_url, commit_id, commit_url }) => {
        const publishState = playgroundState.modes[PlaygroundModes.PUBLISH]
        publishState.url = bundle_url
        publishState.id = bundle_id
        publishState.input = playgroundState.editors.input.getValue()
        publishState.data = playgroundState.editors.data.getValue()
        publishState.policy = data.rego_modules[MODULE_NAME]
        publishState.regoVersionV1Mode = playgroundState.regoVersionV1Mode

        const fetchState = playgroundState.modes[PlaygroundModes.SHARE]
        fetchState.url = commit_url
        fetchState.id = commit_id
        fetchState.input = playgroundState.editors.input.getValue()
        fetchState.data = playgroundState.editors.data.getValue()
        fetchState.policy = data.rego_modules[MODULE_NAME]
        fetchState.regoVersionV1Mode = playgroundState.regoVersionV1Mode

        resolve()
      })
      .catch((error) => reject(error))
  })
}

function handleDropdown({ target }) {
  const menu = target.closest('.dropdown-menu')
  const item = target.closest('.dropdown-item')

  if ((menu && !item) || target.classList.contains('dropdown-item-section')) {
    return
  }

  this.classList.toggle('is-active')

  const content = this.querySelector('.dropdown-content')
  const { top } = content.getBoundingClientRect()
  content.style.maxHeight = `${window.innerHeight - top * 2}px`

  if (this.classList.contains('is-active')) {
    const dropdownMenu = document.querySelector('.dropdown-menu')

    if (!playgroundState.dropdownWidth) {
      playgroundState.dropdownWidth = dropdownMenu.offsetWidth
    }

    adjustPolicyDescriptions()

    const searchInput = dropdownMenu.querySelector('.search-input')
    searchInput.focus()
  }
}

function handleFormat() {
  const formatButton = document.querySelector('#format-button')
  formatButton.classList.add('is-loading')
  hideCoverage()

  const originalOutput = playgroundState.editors.output.getValue()
  let data
  try {
    data = {
      rego_version: playgroundState.regoVersionV1Mode ? 1 : 0,
      rego_module: playgroundState.editors.policy.getValue(),
    }
  } catch (error) {
    displayOutput({
      status: 'Unable to send request.',
      message: '' + error,
      mode: 'text/plain',
    }, 'output')
    formatButton.classList.remove('is-loading')
    return
  }

  let output = { status: '', message: '', mode: 'rego' }
  let requestURL = Api.FORMAT

  fetch(requestURL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSONBigInt.stringify(data)
  }).then(function (resp) {
    return resp.json().then(function (result) {
      // With the response body as parsed JSON.
      if (resp.status === 200) {
        // Clear errors on good formatting.
        if (!/^\d+ error/.test(originalOutput)) {
          output.message = originalOutput
        }
        output.status = `Formatted policy in ${formatNanosecondTime(result.fmt_time)}.`
        // Replace editor text with formatted equivalent, and remove
        // old coverage info.
        playgroundState.editors.policy.setValue(result.result)

        // if the backend was able to use v1 compat for the supplied v0 rego,
        // then we can hide the warning
        if (result.rego_version != 0 && data.rego_version == 0) {
          const warningElem = document.getElementById('v0-upgrade-message')
          if (warningElem) {
            warningElem.classList.add('display-none')
          }
          handleRegoVersionV1Mode(false)
        }
      } else {
        let status = `Error Evaluating.${formatConsoleHint(result)}`
        output = {
          status: status,
          message: result.message,
          mode: 'text/plain',
          trace: result.trace,
        }
      }

      if ('ignored' in result && result.ignored.length > 0) {
        console.log(result.ignored.join('\n'))
      }
    })
  }).catch(function (error) {
    console.log(error)

    output = {
      status: 'Unexpected Error',
      message: `${error}`,
      mode: '',
    }
  }).finally(function () {
    displayOutput(output, 'output')
    formatButton.classList.remove('is-loading')

    // Format the "input" and "data" windows:
    const inputText = playgroundState.editors.input.getValue()
    if (inputText === '') {
      displayOutput({
        status: 'Input was blank. Document will be undefined.',
        message: '',
        mode: 'application/json'
      }, 'input')
    } else {
      try {
        const inputFormatted = JSONBigInt.stringify(JSONBigInt.parse(inputText), null, 4)
        displayOutput({ status: '', message: inputFormatted, mode: 'application/json' }, 'input')
      } catch (error) {
        displayOutput({ status: `${error}`, message: inputText, mode: 'application/json' }, 'input')
      }
    }

    const dataText = playgroundState.editors.data.getValue()
    if (dataText === '') {
      displayOutput({
        status: 'Data was blank. Document will be undefined.',
        message: '',
        mode: 'application/json'
      }, 'data')
    } else {
      try {
        const dataFormatted = JSONBigInt.stringify(JSONBigInt.parse(dataText), null, 4)
        displayOutput({ status: '', message: dataFormatted, mode: 'application/json' }, 'data')
      } catch (error) {
        displayOutput({ status: `${error}`, message: dataText, mode: 'application/json' }, 'data')
      }
    }
  })
  // Remove highlight if the user had selected a line.
  removeLineHighlights(playgroundState.editors.policy)
}

function initButtons() {
  const evalButton = document.querySelector('#evaluate-button')
  evalButton.addEventListener('click', handleEvaluate)

  const strictSwitch = document.querySelector('#strict-switch')
  strictSwitch.addEventListener('click', handleStrictMode)

  const coverageSwitch = document.querySelector('#explain-switch')
  coverageSwitch.addEventListener('click', handleCoverage)

  const regoVersionV0Switch = document.querySelector('#rego-version-v0-switch')
  regoVersionV0Switch.addEventListener('click', handleRegoVersionV0Mode)

  const regoVersionV1Switch = document.querySelector('#rego-version-v1-switch')
  regoVersionV1Switch.addEventListener('click', handleRegoVersionV1Mode)

  const strictBuiltInsSwitch = document.querySelector('#strict-built-in-errors-switch')
  strictBuiltInsSwitch.addEventListener('click', handleStrictBuiltInErrors)

  const allBuiltInErrorsSwitch = document.querySelector('#all-built-in-errors-switch')
  allBuiltInErrorsSwitch.addEventListener('click', handleAllBuiltInErrors)

  const instructionsButton = document.querySelector('#instructions-button')
  instructionsButton.addEventListener('click', handleInstructions)

  const formatButton = document.querySelector('#format-button')
  formatButton.addEventListener('click', handleFormat)

  const publishButton = document.querySelector('#publish-button')
  publishButton.addEventListener('click', handlePublish)

  const defaultBuiltInErrorsSwitch = document.querySelector('#default-built-in-errors-switch')
  defaultBuiltInErrorsSwitch.addEventListener('click', handleDefaultBuiltInErrors)

  const liveLintingSwitch = document.querySelector('#live-linting-switch')
  liveLintingSwitch.addEventListener('click', handleLintMode)

  const authGithubButton = document.querySelector('#auth-github-button')
  authGithubButton.addEventListener('click', handleGithubAuth)

  // Switch checkboxes can have "remembered" states in Firefox, due to the
  // browser saving form-related element state between page refreshes. This will
  // cause the visual presentation of editor state to not match what the code uses.
  // To avoid this, we manually set the UI element state for these elements here.
  updateOptionsFromState(playgroundState)

  if (playgroundState.mode !== PlaygroundModes.PUBLISH) {
    publishButton.setAttribute('disabled', true)
  }

  if (playgroundState.mode !== PlaygroundModes.SHARE) {
    instructionsButton.style.display = 'none'
  } else {
    publishButton.style.display = 'none'
  }

  const modal = document.querySelector('.modal')
  const modalCloseButton = modal.querySelector('.modal-close')
  modalCloseButton.addEventListener('click', closeModal)

  const modalBackground = modal.querySelector('.modal-background')
  modalBackground.addEventListener('click', closeModal)

  for (let button of modal.querySelectorAll('.copy-button')) {
    button.addEventListener('click', () => {
      const valueElement = button.closest('code').querySelector('.copy-value')
      copy(valueElement.textContent)
    })
  }

  document.querySelectorAll('.dropdown').forEach(dropdown => {
    dropdown.addEventListener('click', handleDropdown)
  })

  // if there is a click in the document, close all dropdowns unless the click is within the dropdown
  document.addEventListener('click', (event) => {
    document.querySelectorAll('.dropdown').forEach(dropdown => {
      if (!dropdown.contains(event.target)) {
        dropdown.classList.remove('is-active')
      }
    })
  })

  const dismissedV0WarningButton = document.querySelector('#v0-upgrade-message-dismiss')
  dismissedV0WarningButton.addEventListener('click', () => {
    const v0Warning = document.querySelector('#v0-upgrade-message')
    if (v0Warning) {
      v0Warning.parentNode.removeChild(v0Warning)
    }
  })
}

function handleGithubAuth() {
  try {
    saveStateLocally()
  } catch (error) {
    displayOutput({
      status: 'Unable to save Playground state.',
      message: '' + error,
      mode: 'text/plain',
    }, 'output')
    return
  }

  window.removeEventListener('beforeunload', warnChanges)
  let url = new URL(Api.AUTH, window.location.href)
  window.location.href = url
}

function adjustPolicyDescriptions() {
  if (playgroundState.dropdownWidth) {
    for (let pd of document.querySelectorAll('.policy-description')) {
      pd.style.width = `${playgroundState.dropdownWidth - 32}px`
      pd.style.whiteSpace = 'nowrap'
      pd.style.overflow = 'hidden'
      pd.style.textOverflow = 'ellipsis'

      if (pd.scrollWidth > pd.clientWidth) {
        const policy = pd.closest('a')
        policy.classList.add('has-tooltip-top')
        policy.classList.add('has-tooltip-multiline')
        policy.setAttribute('data-tooltip', pd.textContent)
      }
    }
  }
}

function initPolicies() {
  const dropdownMenu = document.querySelector('.dropdown-menu')
  const tabs = dropdownMenu.querySelector('.panel-tabs')
  const items = dropdownMenu.querySelector('.panel-items')

  function handlePolicy(event) {
    const [typeId, policyId] = event.currentTarget.id.split('.')
    const policyType = policyTypes[typeId]
    const policy = policyType.policies[policyId]

    playgroundState.editors.policy.setValue(versionRegoSyntax(policy.rego))
    playgroundState.editors.input.setValue(policy.input)
    playgroundState.editors.data.setValue(policy.data || '{\n    \n}')
    playgroundState.editors.output.setValue('')

    const { panes } = playgroundState
    if (policy.data) {
      Object.values(panes).forEach((pane) => {
        pane.size = 33
      })
    } else {
      panes[PlaygroundPanes.INPUT].size = 50
      panes[PlaygroundPanes.DATA].size = 0
      panes[PlaygroundPanes.OUTPUT].size = 50
    }

    updateLayout()
  }

  function generatePolicies(typeId, policies, query, section = false) {
    if (section && typeId !== 'all') {
      const header = document.createElement(null)
      header.innerHTML = `
        <div class="dropdown-item dropdown-item-section has-text-weight-semibold">
          ${policyTypes[typeId].name}
        </div>
      `.trim()
      items.appendChild(header.firstChild)
    }

    policies && Object.entries(policies).forEach(([policyId, policy]) => {
      const name = policy.name.toLowerCase()
      const description = policy.description.toLowerCase()
      if (query && `${name} ${description}`.indexOf(query.toLowerCase()) < 0) {
        return
      }

      const id = `${typeId}.${policyId}`
      const container = document.createElement(null)

      container.innerHTML = `
        <a class="dropdown-item" id="${id}">
          <div>${policy.name}</div>
          <div class="is-size-7 has-text-grey policy-description">
            ${policy.description}
          </div>
        </a>
      `.trim()

      const item = container.firstChild
      item.addEventListener('click', handlePolicy)
      items.appendChild(item)

      adjustPolicyDescriptions()
    })
  }

  function filterPolicyType(typeId, policies, query) {
    items.innerHTML = ''

    if (typeId === 'all') {
      Object.entries(policyTypes).forEach(([typeId, { policies }]) => {
        generatePolicies(typeId, policies, query, true)
      })
    } else {
      generatePolicies(typeId, policies, query)
    }
  }

  let currentPolicyType = policyTypes.all
  let query

  orderedPolicyTypes.forEach((typeId) => {
    const policyType = policyTypes[typeId]
    const is_new = policyType.is_new

    const element = document.createElement(null)
    const pill = is_new ? '<span class="new-pill">NEW</span>' : ''
    element.innerHTML = `<a class="panel-tab" id="${typeId}">${policyType.name}${pill}</a>`

    const policyTab = element.firstChild
    tabs.appendChild(policyTab)

    if (typeId === 'all') {
      policyTab.classList.add('is-active')
    }

    policyTab.addEventListener('click', (event) => {
      for (let tab of tabs.children) {
        tab.classList.remove('is-active')
      }

      policyTab.classList.add('is-active')
      currentPolicyType = policyTypes[event.target.id]
      filterPolicyType(currentPolicyType.id, currentPolicyType.policies, query)
    })

    generatePolicies(typeId, policyType.policies, undefined, true)
  })

  const searchInput = dropdownMenu.querySelector('.search-input')
  searchInput.addEventListener('input', (event) => {
    query = event.target.value
    filterPolicyType(currentPolicyType.id, currentPolicyType.policies, query)
  })
}

// loadStateParam sets the playground state based on state 'baked' into the state parameter
function loadStateParam() {
  // only run on the root page
  if (window.location.pathname !== '/') { return }

  const params = new URLSearchParams(window.location.search)
  const logStateLink = params.get('log_state_link')
  if (logStateLink !== null) {
    playgroundState.logStateLink = true
    return
  }

  const state = params.get('state')
  if (state === null) {
    return
  }

  const stateData = JSON.parse(decodeFromBase64(state))

  if (stateData.p !== undefined) {
    playgroundState.editors.policy.setValue(stateData.p)
  }

  if (stateData.i !== undefined) {
    playgroundState.editors.input.setValue(stateData.i)
  }

  if (stateData.d !== undefined) {
    playgroundState.editors.data.setValue(stateData.d)
    // enough to show that there is some data set (3 lines on a small window)
    playgroundState.panes[PlaygroundPanes.DATA].size = 20
  }

  if (stateData.s !== undefined) {
    playgroundState.strictMode = stateData.s
  }

  if (stateData.r !== undefined) {
    playgroundState.regoVersionV1Mode = stateData.r
  }

  if (stateData.c !== undefined) {
    playgroundState.showCoverage = stateData.c
  }

  if (stateData.b !== undefined) {
    if (stateData.b === 'all') {
      handleAllBuiltInErrors()
    } else if (stateData.b === 'strict') {
      handleStrictBuiltInErrors()
    }
  }

  handleFormat()
  handleEvaluate()

  window.history.replaceState(null, document.title, '/')
}

// initLogStateLink enables the log state link behavior if param 'logStateLink' is set.
// This is intended to be used for testing and debugging purposes.
function initLogStateLink() {
  // only run on the root page
  if (window.location.pathname !== '/') { return }

  const params = new URLSearchParams(window.location.search)

  const logStateLink = params.get('log_state_link')
  if (logStateLink !== null) {
    playgroundState.logStateLink = true
  }
}

// Encodes a UTF-16 JS string to a base64 UTF-8 string
function encodeToBase64(str) {
  const utf8Bytes = new TextEncoder().encode(str)
  const base64String = btoa(String.fromCharCode.apply(null, utf8Bytes))
  return base64String
}

// Decodes a base64 UTF-8 string to a UTF-16 JS string
function decodeFromBase64(base64String) {
  const binaryString = atob(base64String)
  const utf8Bytes = Uint8Array.from(binaryString, char => char.charCodeAt(0))
  const decodedString = new TextDecoder().decode(utf8Bytes)
  return decodedString
}

// logStateLink logs a link 'baking' the current playground state into the URL
function logStateLink() {
  let state = {
    p: playgroundState.editors.policy.getValue(),
    i: playgroundState.editors.input.getValue(),
    d: playgroundState.editors.data.getValue(),
    c: playgroundState.showCoverage,
    s: playgroundState.strictMode,
    r: playgroundState.regoVersionV1Mode,
  }

  if (playgroundState.strictBuiltInErrors) {
    state.b = 'strict'
  } else if (playgroundState.allBuiltInErrors) {
    state.b = 'all'
  }

  let url = new URL(window.location.href)
  url.search = new URLSearchParams({
    state: encodeToBase64(JSON.stringify(state)),
  }).toString()

  // https://stackoverflow.com/a/417184/21939446
  if (url.toString().length > 2000) {
    ('state link too long to be compatible with all browsers, please use Publish feature instead')
  }

  console.log(url.toString())
}

// selectExampleGroup selects the example group based on the URL query parameter 'example-group'
// from the dropdown of example policies
function selectExampleGroup() {
  const params = new URLSearchParams(window.location.search)
  if (params.get('example-group') !== null) {
    const exampleGroup = params.get('example-group')
    if (exampleGroup === null) { return }

    const dd = document.getElementById('dropdown-examples')
    if (dd === null) { return }
    dd.classList.add('is-active')

    const eg = document.getElementById(exampleGroup)
    if (eg === null) { return }

    eg.click()
  }
}

function warnChanges(e) {
  if (hasChanges()) {
    // Cancel the event
    e.preventDefault()
    e.returnValue = ''
  } else {
    delete e['returnValue']
  }
}

function getLineFromHash() {
  const m = window.location.hash.match(/^#L(\d+)$/)
  return m ? parseInt(m[1], 10) : null
}

function initialize() {
  // Unhide the playground content
  document.querySelector('#playground-container').removeAttribute('hidden')

  // Setup main policy editing codemirror
  initEditors()

  // Lint the policy on start-up
  handleLint()

  // Initialize buttons and related event handlers
  initButtons()

  // Setup policies in dropdown
  initPolicies()

  // set the policy, input and data editors if state param included
  loadStateParam()

  // enable the log state link behavior if param is included
  initLogStateLink()

  // Call updateLayout once to initialize everything to the default state
  updateLayout()

  // Fill in the OPA version
  loadVersionInfo()

  loadRegoVersionInfo()

  // if ?example-group is set, select the group and open the dropdown
  selectExampleGroup()

  window.addEventListener('beforeunload', warnChanges)

  playgroundState.editors.policy.focus()

  window.onresize = debounce(updateLayout, 100)
}

function showDenied() {
  let modal = document.querySelector('.modal')
  modal.querySelector('.denied-level').style.display =  'initial'
  // if auth is set to required, show modal and Github auth button
  showModal('123', '', true, 'NEW') // same as showModal but with the old share disabled

  // hide URL not ready yet
  modal.querySelector('.status-tag').style.display =  'none'
  modal.querySelector('#share-url').style.display =  'none'
  modal.querySelector('#gist-url').style.display =  'none'
  modal.querySelector('#run-opa').style.display =  'none'
  modal.querySelector('#query-opa').style.display =  'none'
}

(() => {
  // Expecting a url like <protocol>://<host>/<path>/p/<key> (or with `d` instead of `p`)
  // with the second match group being the key..
  const url = window.location.href
  const sharedMatch = url.match(RegexKeys[PlaygroundModes.SHARE])

  const params = new URLSearchParams(window.location.search)

  const match = {}
  if (sharedMatch && sharedMatch.length === 3) {
    match.mode = PlaygroundModes.SHARE
    match.id = sharedMatch[2]
  } else {
    // No match for the key in the url
    initialize()

    let rego_version = sessionStorage.getItem('rego_version')
    if (rego_version != null) {
      sessionStorage.removeItem('rego_version')
      playgroundState.regoVersionV1Mode = rego_version
    }
    let policy = sessionStorage.getItem('policy')
    if (policy != null) {
      sessionStorage.removeItem('policy')
      playgroundState.editors.policy.setValue(policy)
    }
    let input = sessionStorage.getItem('input')
    if (input != null) {
      playgroundState.editors.input.setValue(input)
      sessionStorage.removeItem('input')
    }
    let data = sessionStorage.getItem('data')
    if (data != null) {
      playgroundState.editors.data.setValue(data)
      sessionStorage.removeItem('data')
    }

    playgroundState.mode = PlaygroundModes.SHARE

    // Check if this is a callback from auth
    const parsedHash = new URLSearchParams(
      window.location.hash.substring(1) // any_hash_key=any_value
    )

    let auth = parsedHash.get('auth')
    const modes = sessionStorage.getItem('modes')

    switch (auth) {
      case 'denied':
        showDenied()
        break
      case 'success':
        if (modes !== null) {
          playgroundState.modes = JSON.parse(modes)
        }

        document.querySelector('.denied-level').style.display =  'none'
        document.querySelector('.new-share-level').style.display =  'initial'

        handleShowPublishedGist()

        break
    }

    return
  }

  let reqURL = `${Api.DATA}/${match.id}`

  if (params !== null) {
    reqURL = `${Api.DATA}/${match.id}?${params}`
  }

  fetch(reqURL)
    .then(async(response) => {
      if (response.ok) {
        return response.text()
      } else {
        // if user isn't authenticated, try to load gist before forcing the user to log in
        if (response.status == 401) {
          let decoded
          try {
            decoded = await decodeKey(match.id)
          } catch (e) {
            console.log(e)
            throw Error('failed to decode share key')
          }

          let data = await getGist(decoded.id, decoded.revision)
          if (data == null) {
            // TODO allow user to auth
            throw Error('rate limited, please try again later')
          } else {
            return JSON.stringify(data)
          }
        }

        throw Error(response.statusText)
      }
    })
    .then((data) => {
      return JSONBigInt.parse(data)
    })
    .then(({ input, value, data, coverage, result, eval_time, rego_version }) => {
      const state = playgroundState.modes[match.mode]
      state.policy = value
      state.input = JSONBigInt.stringify(input, null, 4)
      state.url = url

      if (match.mode === PlaygroundModes.SHARE) {
        state.strict = true
        if (params.has('strict')) {
          state.strict = params.get('strict') === 'true'
        }

        state.coverage = false
        if (params.has('coverage')) {
          if (params.get('coverage') === 'true') {
            state.coverage = true
            playgroundState.currentCoverage = coverage
          }
        }

        if (params.has('evaluate')) {
          if (params.get('evaluate') === 'true') {
            let formatted = formatOutput({ 'result': result, 'eval_time': eval_time })
            state.output = {
              status: formatted.comment,
              message: formatted.message,
              mode: 'rego',
            }
          }
        }
      }

      if (data && Object.keys(data).length > 0) {
        state.data = JSONBigInt.stringify(data, null, 4)
      }

      state.id = match.id
      playgroundState.mode = match.mode
      playgroundState.regoVersionV1Mode = rego_version
      initialize()
    })
    .catch((error) => {
      initialize()

      displayOutput({
        status: 'Unexpected error loading saved data',
        message: `${error}`,
        mode: '',
      }, 'output')
    })
})()
