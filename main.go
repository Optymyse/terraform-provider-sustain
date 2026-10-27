// Sustain's provider for Terraform and OpenTofu (README.md).
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/optymyse/terraform-provider-sustain/internal/provider"
)

// version is the release's, set as it's built (.goreleaser.yml).
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "Run the provider for a debugger, which Terraform then connects to.")
	flag.Parse()
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/optymyse/sustain",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
