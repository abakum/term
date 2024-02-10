# term - utilities for dealing with terminals

![Test](https://github.com/abakum/term/workflows/Test/badge.svg) [![GoDoc](https://godoc.org/github.com/abakum/term?status.svg)](https://godoc.org/github.com/abakum/term) [![Go Report Card](https://goreportcard.com/badge/github.com/abakum/term)](https://goreportcard.com/report/github.com/abakum/term)

term provides structures and helper functions to work with terminal (state, sizes).

#### Using term

```go
package main

import (
	"log"
	"os"

	"github.com/abakum/term"
)

func main() {
	fd := os.Stdin.Fd()
	if term.IsTerminal(fd) {
		ws, err := term.GetWinsize(fd)
		if err != nil {
			log.Fatalf("term.GetWinsize: %s", err)
		}
		log.Printf("%d:%d\n", ws.Height, ws.Width)
	}
}
```

## Contributing

Want to hack on term? [Docker's contributions guidelines](https://github.com/docker/docker/blob/master/CONTRIBUTING.md) apply.

## Copyright and license
Code and documentation copyright 2015 Docker, inc. Code released under the Apache 2.0 license. Docs released under Creative commons.
