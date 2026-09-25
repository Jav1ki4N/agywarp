package main

import (
	"os"

	"agywarp/internal/cli"
)

/* +~~~~~~~~~~~~~~~~~~~~~~~~~~+ */
/*           main.go            */
/* +~~~~~~~~~~~~~~~~~~~~~~~~~~+ */
/*        Package: main         */
/*  The entrypoint of agywarp   */
/* Copyright 2026 (C) Ian Javik */
/* +~~~~~~~~~~~~~~~~~~~~~~~~~~+ */

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
