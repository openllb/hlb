package main

import (
	"log"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb/naive"
)

func main() {
	if err := grpcclient.RunFromEnvironment(appcontext.Context(), naive.Build); err != nil {
		log.Printf("fatal error: %+v", err)
		panic(err)
	}
}
