package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"

	"github.com/dgraph-io/badger"

	"github.com/nclv/golang-blockchain/blockchain"
	"github.com/nclv/golang-blockchain/network"
	"github.com/nclv/golang-blockchain/wallet"
)

type CommandLine struct{}

func (cli *CommandLine) PrintUsage() {
	fmt.Println("Usage:")
	fmt.Println(" getbalance -address ADDRESS - get the balance for the address")
	fmt.Println(" createblockchain -address ADDRESS - creates a blockchain and send genesis reward to address")
	fmt.Println(" printchain - Prints the blocks in the chain")
	fmt.Println(" send -from FROM -to TO -amount AMOUNT -mine - Send an amount of coins. Then -mine flag is set.")
	fmt.Println(" createwallet - Creates a new Wallet")
	fmt.Println(" listaddresses - Lists the addresses in our wallet file")
	fmt.Println(" reindexutxo - Rebuilds the UTXO set")
	fmt.Println(" startnode -miner ADDRESS - Start a node with ID specified in NODE_ID env. var. -miner enables mining")
}

func (cli *CommandLine) ValidateArgs() {
	if len(os.Args) < 2 {
		cli.PrintUsage()
		runtime.Goexit()
	}
}

func (cli *CommandLine) StartNode(nodeID, minerAddress string) {
	fmt.Printf("Starting Node %s\n", nodeID)

	if len(minerAddress) > 0 {
		if wallet.ValidateAddress(minerAddress) {
			fmt.Println("Mining is on. Address to receive rewards: ", minerAddress)
		} else {
			log.Panic("Wrong miner address!")
		}
	}
	network.StartServer(nodeID, minerAddress)
}

func (cli *CommandLine) ListAddresses(nodeID string) {
	wallets, _ := wallet.CreateWallets(nodeID)
	addresses := wallets.GetAllAddresses()

	for _, address := range addresses {
		fmt.Println(address)
	}
}

func (cli *CommandLine) CreateWallet(nodeID string) {
	wallets, _ := wallet.CreateWallets(nodeID)
	address := wallets.AddWallet()
	wallets.SaveFile(nodeID)

	fmt.Printf("New address is: %s\n", address)
}

func (cli *CommandLine) ReindexUTXO(nodeID string) {
	chain := blockchain.ContinueBlockChain(nodeID)
	defer func(Database *badger.DB) {
		err := Database.Close()
		if err != nil {
			log.Panic(err)
		}
	}(chain.Database)

	UTXOSet := blockchain.UTXOSet{BlockChain: chain}
	UTXOSet.Reindex()

	count := UTXOSet.CountTransactions()
	fmt.Printf("Done! There are %d transactions in the UTXO set.\n", count)
}

func (cli *CommandLine) PrintChain(nodeID string) {
	chain := blockchain.ContinueBlockChain(nodeID)
	defer func(Database *badger.DB) {
		err := Database.Close()
		if err != nil {
			log.Panic(err)
		}
	}(chain.Database)

	iter := chain.Iterator()
	for {
		block := iter.Next()

		fmt.Printf("Previous Hash: %x\n", block.PrevHash)
		fmt.Printf("Hash: %x\n", block.Hash)

		pow := blockchain.NewProof(block)
		fmt.Printf("PoW: %s\n", strconv.FormatBool(pow.Validate()))
		for _, tx := range block.Transactions {
			fmt.Println(tx)
		}
		fmt.Println()

		if len(block.PrevHash) == 0 {
			break
		}
	}
}

func (cli *CommandLine) CreateBlockChain(address, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not valid")
	}

	chain := blockchain.InitBlockChain(address, nodeID)
	defer func(Database *badger.DB) {
		err := Database.Close()
		if err != nil {
			log.Panic(err)
		}
	}(chain.Database)

	UTXOSet := blockchain.UTXOSet{BlockChain: chain}
	UTXOSet.Reindex()

	fmt.Println("Finished!")
}

func (cli *CommandLine) GetBalance(address, nodeID string) {
	if !wallet.ValidateAddress(address) {
		log.Panic("Address is not valid")
	}

	chain := blockchain.ContinueBlockChain(nodeID)
	UTXOSet := blockchain.UTXOSet{BlockChain: chain}
	defer func(Database *badger.DB) {
		err := Database.Close()
		if err != nil {
			log.Panic(err)
		}
	}(chain.Database)

	balance := 0
	pubKeyHash := wallet.Base58Decode([]byte(address))
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-4]
	UTXOs := UTXOSet.FindUnspentTransactions(pubKeyHash)
	for _, out := range UTXOs {
		balance += out.Value
	}

	fmt.Printf("Balance of %s: %d\n", address, balance)
}

// Send from is the user mining the transaction
func (cli *CommandLine) Send(from, to string, amount int, nodeID string, mineNow bool) {
	if !wallet.ValidateAddress(from) {
		log.Panic("Address is not valid")
	}
	if !wallet.ValidateAddress(to) {
		log.Panic("Address is not valid")
	}

	chain := blockchain.ContinueBlockChain(nodeID)
	UTXOSet := blockchain.UTXOSet{BlockChain: chain}
	defer func(Database *badger.DB) {
		err := Database.Close()
		if err != nil {
			log.Panic(err)
		}
	}(chain.Database)

	wallets, err := wallet.CreateWallets(nodeID)
	if err != nil {
		log.Panic(err)
	}
	wallet := wallets.GetWallet(from)

	tx := blockchain.NewTransaction(&wallet, to, amount, &UTXOSet)
	if mineNow {
		cbTx := blockchain.CoinbaseTx(from, "")
		txs := []*blockchain.Transaction{cbTx, tx}
		block := chain.MineBlock(txs)
		UTXOSet.Update(block)
	} else {
		network.SendTx(network.KnownNodes[0], tx)
		fmt.Println("Send tx")
	}

	fmt.Println("Success!")
}

func (cli *CommandLine) Run() {
	cli.ValidateArgs()

	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Println("NODE_ID env is not set!")
		runtime.Goexit()
	}

	getBalanceCmd := flag.NewFlagSet("getbalance", flag.ExitOnError)
	createBlockchainCmd := flag.NewFlagSet("createblockchain", flag.ExitOnError)
	sendCmd := flag.NewFlagSet("send", flag.ExitOnError)
	printChainCmd := flag.NewFlagSet("print", flag.ExitOnError)
	createWalletCmd := flag.NewFlagSet("createwallet", flag.ExitOnError)
	listAddressesCmd := flag.NewFlagSet("listaddresses", flag.ExitOnError)
	reindexUTXOCmd := flag.NewFlagSet("reindexutxo", flag.ExitOnError)
	startNodeCmd := flag.NewFlagSet("startnode", flag.ExitOnError)

	getBalanceAddress := getBalanceCmd.String("address", "", "The name of the account")
	createBlockchainAddress := createBlockchainCmd.String("address", "", "The name of the account")
	sendFrom := sendCmd.String("from", "", "Source wallet address")
	sendTo := sendCmd.String("to", "", "Destination wallet address")
	sendAmount := sendCmd.Int("amount", 0, "Amount to send")
	sendMine := sendCmd.Bool("mine", false, "Mine immediately on the same node")
	startNodeMiner := startNodeCmd.String("miner", "", "Enable mining mode and send reward")

	switch os.Args[1] {
	case "getbalance":
		if err := getBalanceCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "listaddresses":
		if err := listAddressesCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "createwallet":
		if err := createWalletCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "reindexutxo":
		if err := reindexUTXOCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "createblockchain":
		if err := createBlockchainCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "printchain":
		if err := printChainCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "send":
		if err := sendCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	case "startnode":
		if err := startNodeCmd.Parse(os.Args[2:]); err != nil {
			log.Panic(err)
		}
	default:
		cli.PrintUsage()
		runtime.Goexit()
	}

	if getBalanceCmd.Parsed() {
		if *getBalanceAddress == "" {
			getBalanceCmd.Usage()
			runtime.Goexit()
		}
		cli.GetBalance(*getBalanceAddress, nodeID)
	}

	if createBlockchainCmd.Parsed() {
		if *createBlockchainAddress == "" {
			createBlockchainCmd.Usage()
			runtime.Goexit()
		}
		cli.CreateBlockChain(*createBlockchainAddress, nodeID)
	}

	if printChainCmd.Parsed() {
		cli.PrintChain(nodeID)
	}
	if createWalletCmd.Parsed() {
		cli.CreateWallet(nodeID)
	}
	if reindexUTXOCmd.Parsed() {
		cli.ReindexUTXO(nodeID)
	}
	if listAddressesCmd.Parsed() {
		cli.ListAddresses(nodeID)
	}
	if startNodeCmd.Parsed() {
		nodeID := os.Getenv("NODE_ID")
		if nodeID == "" {
			startNodeCmd.Usage()
			runtime.Goexit()
		}
		cli.StartNode(nodeID, *startNodeMiner)
	}

	if sendCmd.Parsed() {
		if *sendFrom == "" || *sendTo == "" || *sendAmount <= 0 {
			sendCmd.Usage()
			runtime.Goexit()
		}
		cli.Send(*sendFrom, *sendTo, *sendAmount, nodeID, *sendMine)
	}
}
