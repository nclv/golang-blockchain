package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"

	"github.com/nclv/golang-blockchain/blockchain"
)

type CommandLine struct {
	blockchain *blockchain.BlockChain
}

func (cli *CommandLine) PrintUsage() {
	fmt.Println("Usage:")
	fmt.Println(" add -block <block_data> - Add a block to the chain")
	fmt.Println(" print - Prints the blocks in the chain")
}

func (cli *CommandLine) ValidateArgs() {
	if len(os.Args) < 2 {
		cli.PrintUsage()
		runtime.Goexit()
	}
}

func (cli *CommandLine) AddBlock(data string) {
	cli.blockchain.AddBlock(data)
	fmt.Println("Block added.")
}

func (cli *CommandLine) PrintChain() {
	iter := cli.blockchain.Iterator()

	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Data: %s\n", block.Data)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) Run() {
	cli.ValidateArgs()

	addBlockCmd := flag.NewFlagSet("add", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("print", flag.ExitOnError)
	addBlockData := addBlockCmd.String("block", "", "Block Data")

	switch os.Args[1] {
	case "add":
		if err := addBlockCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "print":
		if err := printChainCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	default:
		cli.PrintUsage()
		runtime.Goexit()
	}

	if addBlockCmd.Parsed() {
		if *addBlockData == "" {
			addBlockCmd.Usage()
			runtime.Goexit()
		}
		cli.AddBlock(*addBlockData)
	}

	if printChainCmd.Parsed() {
		cli.PrintChain()
	}
}

func main() {
	defer os.Exit(0)

	chain := blockchain.InitBlockChain()
	defer chain.Database.Close()

	cli := CommandLine{chain}
	cli.Run()
}
