# Rego Playground

This repository contains the backend server and web ui for the **Rego Playground**.

# Architecture
## Overview
The playground consists of a Golang server with REST API's along with
a pretty bare-bones web UI. The playground server is built as a single
binary and the web content that it serves must be deployed along with it.

Storage for the server can be handled either in-memory with no
persistence or backed by S3 and/or GitHub Gists.

## Golang Server
The main entry point is in [./cmd/rego-playground/main.go](./cmd/rego-playground/main.go)
and then the main server implementation is in [./api](./api).

### Playground Share Links

A playground share link is of the form `GET /p/<key>`. The following query parameters are supported:

* **strict** - If parameter is `true`, strict-mode is enabled. Default: `true`
* **coverage** -  If parameter is `true`, coverage will be displayed. Default: `false`
* **evaluate** -  If parameter is `true`, policy will be evaluated and output displayed. Default: `false`

## Web UI
The web UI is in [./ui](./ui) and built using `npm` modules and `webpack`.
This is primarily due to the dependency we have on the `codemirror-rego`
npm module. The UI uses a couple of helpers, but generally
avoids any heavyweight frameworks.

# Directory Layout
`./api`, `./cmd`, `./opa`, `./presentation`, `./utils` -- Are the Golang server packages

`./ui` -- Contains the web frontent code

`./scripts` -- Helper scripts for building & testing

# Building
## Production build
`make all`

This will clean, run tests, install UI deps, build the UI, and build the server binary.

## Build the Server
`make rego-playground`

Will build the playground binary. The binary can be found at `./build/rego-playground`.

## Build the UI
`make ui-deps`

Must be run once to install the `npm` dependencies

`make ui-prod`

Will use webpack to build the production web content. Output can be found in `./build/ui`.

### Development workflow
Use the `ui-dev-watch` target to build the ui content in development mode _and_
have it watch for changes. This allows for starting the backend server and then
seeing live updates to the content as you make changes to the html/css/javascript/etc.

Note that any changes to `webpack.config.js` require restarting the watch.

Once this is started just run the backend server locally with the instructions below...

# Running the Playground
## Locally (no S3)

### Easy Mode

```
make run-rego-playground-dev
```

This does a `go run` with the dev [config file](./dev-config.yaml)

### Manual Approach

First build the playground (or use the following args w/ `go run`).

To run the playground locally without any S3 access use the following parameters:

```bash
./build/rego-playground --ui-content-root ./build/ui --no-persist --external-url http://localhost:8181
```

This will run the playground with..
* A content root set to `./build/ui` so that the frontend files can be served.
* No persistence to S3 (inmem store only)
* An external url set to http://localhost:8181 so that shared links will work correctly

You can now access the playground by opening [http://localhost:8181/](http://localhost:8181)
the playground will have full functionality, but will lose shared content upon restart.

# Updating/Adding Dependencies
## Go deps
The project is setup as a Go module. To update do something like:

`go get -u <pkg>@<version>`

Then make sure it works.. and before committing run

`go mod tidy && go mod vendor`

Which will ensure dependencies are still all good and the local copy in
`./vendor` is up to date.

### Updating OPA
There is a helper script and make target for this.. just run

```bash
TAG=<version> make update-opa
```

This is for local experiments. Usually, dependabot will take care of updating
OPA.

## NPM Modules
Just use normal `npm install ..` commands, but be sure to only use,
`--save-dev`/`-D`. The `ui` directory is *not* a module that can/should
be exported. We only use the node modules for development purposes.

Ex:

```
npm i -D <pkg>
```

If updating a module you can just update the `./ui/package.json` file
and run `make ui-deps` to ensure it is installed.
