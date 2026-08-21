// Command isore dispatches to its subcommands: `mcp` runs the MCP server
// (whose start_proxy tool fronts the user's dev server with the annotation
// overlay and drives a real Chromium for the browser_* tools), and
// `subscribe` is a long-lived CLI listener that streams dispatched annotations
// from the proxy's SSE stream — run as a single background command so the
// agent is notified of every dispatch without holding a blocking tool call.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: isore <mcp|subscribe> [flags]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "mcp":
		if err := runMCP(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "isore mcp:", err)
			os.Exit(1)
		}
	case "subscribe":
		if err := runSubscribe(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "isore subscribe:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "isore: unknown subcommand %q (want mcp or subscribe)\n", os.Args[1])
		os.Exit(1)
	}
}
