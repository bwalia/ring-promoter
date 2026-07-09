// Placeholder module: this directory is the Node.js frontend, not Go code.
// Its presence makes the root module's `go build ./...` / `go test ./...`
// skip the whole web/ tree (node_modules can contain stray .go files).
module github.com/example/ring-promoter/web

go 1.25
