// Command bank-api is the entrypoint for the high-governance Ring Promoter
// training app. All logic lives in the parent bankapi package (kept at the
// module root so it can //go:embed the db/migrations SQL); this wrapper only
// links the build-time version and starts the server.
package main

import (
	"log"
	"os"

	bankapi "github.com/bwalia/ring-promoter/training/bank-api"
)

// version is baked in at build time: go build -ldflags "-X main.version=v1.2.3".
// At runtime RP_VERSION wins (see bankapi.Run) so the image reports the tag it
// was deployed as, which /healthz echoes for Ring Promoter's version check.
var version = "dev"

func main() {
	// `bank-api migrate` runs the chart's migrate Job step: apply the embedded
	// migrations and exit. Any other invocation starts the HTTP server.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := bankapi.Migrate(); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		return
	}
	if err := bankapi.Run(version); err != nil {
		log.Fatal(err)
	}
}
