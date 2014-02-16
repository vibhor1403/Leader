package main

import (
	"github.com/vibhor1403/Leader/Raft"
	"os"
	"flag"
)

func main() {

	// parse argument flags and get this server's id into myid
	mypid := flag.Int("pid", 0, "Pid of my own system")
	flag.Parse()
	_ = Raft.New(*mypid /* config file */, "./config.json")

	// wait for keystroke.
	var input string
	fmt.Scanln(&input)
}

