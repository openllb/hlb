package main

import (
	"log"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/openllb/hlb"
)

func main() {
	if err := grpcclient.RunFromEnvironment(appcontext.Context(), hlb.Frontend); err != nil {
		log.Printf("fatal error: %+v", err)
		panic(err)
	}
}
