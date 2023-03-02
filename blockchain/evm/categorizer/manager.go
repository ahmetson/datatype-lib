// EVM blockchain worker's manager
// For every blockchain we have one manager.
// Manager keeps the list of the smartcontract workers:
// - list of workers for up to date smartcontracts
// - list of workers for categorization outdated smartcontracts
package categorizer

import (
	app_log "github.com/blocklords/gosds/app/log"
	"github.com/charmbracelet/log"

	"time"

	"github.com/blocklords/gosds/app/service"
	blockchain_proc "github.com/blocklords/gosds/blockchain/inproc"
	"github.com/blocklords/gosds/blockchain/network"
	"github.com/blocklords/gosds/categorizer"

	"github.com/blocklords/gosds/blockchain/evm/abi"
	"github.com/blocklords/gosds/categorizer/smartcontract"
	"github.com/blocklords/gosds/common/data_type"
	static_abi "github.com/blocklords/gosds/static/abi"

	"github.com/blocklords/gosds/app/argument"
	"github.com/blocklords/gosds/app/remote/message"
	spaghetti_log "github.com/blocklords/gosds/blockchain/event"
	spaghetti_block "github.com/blocklords/gosds/blockchain/evm/block"
	zmq "github.com/pebbe/zmq4"

	"github.com/blocklords/gosds/app/remote"
)

const IDLE = "idle"
const RUNNING = "running"

// Categorization of the smartcontracts on the specific EVM blockchain
type Manager struct {
	pusher  *zmq.Socket
	Network *network.Network

	logger log.Logger

	old_categorizers OldWorkerGroups

	current_workers EvmWorkers

	subscribed_earliest_block_number uint64
	subscribed_blocks                data_type.Queue
}

// Creates a new manager for the given EVM Network
// New manager runs in the background.
func NewManager(logger log.Logger, network *network.Network) *Manager {
	categorizer_logger := app_log.Child(logger, "categorizer")

	manager := Manager{
		Network: network,

		old_categorizers: make(OldWorkerGroups, 0),

		subscribed_blocks:                *data_type.NewQueue(),
		subscribed_earliest_block_number: 0,

		// consumes the data from the subscribed blocks
		current_workers: make(EvmWorkers, 0),

		logger: categorizer_logger,
	}

	return &manager
}

// Returns all smartcontracts from all types of workers
func (manager *Manager) GetSmartcontracts() []*smartcontract.Smartcontract {
	smartcontracts := make([]*smartcontract.Smartcontract, 0)

	for _, group := range manager.old_categorizers {
		smartcontracts = append(smartcontracts, group.workers.GetSmartcontracts()...)
	}

	smartcontracts = append(smartcontracts, manager.current_workers.GetSmartcontracts()...)

	return smartcontracts
}

func (manager *Manager) GetSmartcontractAddresses() []string {
	addresses := make([]string, 0)

	for _, group := range manager.old_categorizers {
		addresses = append(addresses, group.workers.GetSmartcontractAddresses()...)
	}

	addresses = append(addresses, manager.current_workers.GetSmartcontractAddresses()...)

	return addresses
}

// Same as Run. Run it as a goroutine
func (manager *Manager) Start() {
	manager.logger.Info("starting categorization")
	go manager.subscribe()

	// wait until we receive the new block number
	for {
		if manager.subscribed_earliest_block_number == 0 {
			time.Sleep(time.Second * 1)
			continue
		}
		break
	}

	manager.logger.Info("subscription started")
	go manager.categorize_current_smartcontracts()

	sock, err := zmq.NewSocket(zmq.PULL)
	if err != nil {
		manager.logger.Fatal("new manager pull socket", "message", err)
	}

	url := blockchain_proc.CategorizerManagerUrl(manager.Network.Id)
	if err := sock.Bind(url); err != nil {
		log.Fatal("trying to create categorizer for network id %s: %v", manager.Network.Id, err)
	}

	// if there are some logs, we should broadcast them to the SDS Categorizer
	pusher, err := categorizer.NewCategorizerPusher()
	if err != nil {
		manager.logger.Fatal("create a pusher to SDS Categorizer", "message", err)
	}
	manager.pusher = pusher

	manager.logger.Info("waiting for messages")

	for {
		// Wait for reply.
		msgs, _ := sock.RecvMessage(0)
		request, _ := message.ParseRequest(msgs)

		raw_smartcontracts, _ := request.Parameters.GetKeyValueList("smartcontracts")
		raw_abis, _ := request.Parameters["abis"].([]interface{})

		new_workers := make(EvmWorkers, len(raw_abis))

		for i, raw_abi := range raw_abis {
			abi_data, _ := static_abi.New(raw_abi.(map[string]interface{}))
			cat_abi, _ := abi.NewAbi(abi_data)

			sm, _ := smartcontract.New(raw_smartcontracts[i])

			manager.logger.Info("add a new worker", "number", i+1, "total", len(new_workers))
			new_workers[i] = New(sm, cat_abi)
		}

		block_number := manager.subscribed_earliest_block_number

		manager.logger.Info("information about workers", "block_number", block_number, "amount of workers", len(new_workers))

		old_workers, current_workers := new_workers.Sort().Split(block_number)
		old_block_number := old_workers.EarliestBlockNumber()

		manager.logger.Info("splitting to old and new workers", "old amount", len(old_workers), "new amount", len(current_workers))
		manager.logger.Info("old workers information", "earliest_block_number", old_block_number)

		group := manager.old_categorizers.FirstGroupGreaterThan(old_block_number)
		if group == nil {
			manager.logger.Info("create a new group of old workers")
			group = NewGroup(old_block_number, old_workers)
			manager.old_categorizers = append(manager.old_categorizers, group)
			go manager.categorize_old_smartcontracts(group)
		} else {
			manager.logger.Info("add to the existing group")
			group.add_workers(old_workers)
		}

		manager.logger.Info("add current workers")

		manager.add_current_workers(current_workers)
	}
}

// Categorization of the smartcontracts that are super old.
//
// Get List of smartcontract addresses
// Get Log for the smartcontracts.
func (manager *Manager) categorize_old_smartcontracts(group *OldWorkerGroup) {
	old_logger := app_log.Child(manager.logger, "old_logger_"+time.Now().String())

	url := blockchain_proc.BlockchainManagerUrl(manager.Network.Id)
	blockchain_socket := remote.InprocRequestSocket(url)
	defer blockchain_socket.Close()

	old_logger.Info("starting categorization of old smartcontracts.", "blockchain client manager", url)

	for {
		block_number_from := group.block_number + uint64(1)
		addresses := manager.GetSmartcontractAddresses()

		old_logger.Info("fetch from blockchain client manager logs", "block_number", block_number_from, "addresses", addresses)

		all_logs, err := spaghetti_log.RemoteLogFilter(blockchain_socket, block_number_from, addresses)
		if err != nil {
			old_logger.Warn("SKIP, blockchain manager returned an error for block number %d and addresses %v: %w", block_number_from, addresses, err)
			continue
		}

		old_logger.Info("fetched from blockchain client manager", "logs amount", len(all_logs))

		// update the worker data by logs.
		block_number_to := block_number_from
		for _, worker := range group.workers {
			logs := spaghetti_log.FilterByAddress(all_logs, worker.smartcontract.Address)
			if len(logs) == 0 {
				continue
			}
			categorized_logs, recent_block_number := worker.categorize(logs)
			block_number_to = recent_block_number

			smartcontracts := []*smartcontract.Smartcontract{worker.smartcontract}

			push := message.Request{
				Command: "",
				Parameters: map[string]interface{}{
					"smartcontracts": smartcontracts,
					"logs":           categorized_logs,
				},
			}
			request_string, _ := push.ToString()

			old_logger.Info("send to SDS Categorizer", "logs amount", len(logs))

			_, err = manager.pusher.SendMessage(request_string)
			if err != nil {
				old_logger.Fatal("send to SDS Categorizer", "message", err)
			}
		}

		left := block_number_to - group.block_number
		old_logger.Info("categorized certain blocks", "block_number_left", left)
		group.block_number = block_number_to

		if block_number_to >= manager.subscribed_earliest_block_number {
			old_logger.Info("catched the current blocks")
			manager.add_current_workers(group.workers)
			break
		}
	}
	// delete the categorizer group
	manager.old_categorizers = manager.old_categorizers.Delete(group)

	old_logger.Info("finished!")
}

// Move recent to consuming
func (manager *Manager) add_current_workers(workers EvmWorkers) {
	manager.current_workers = append(manager.current_workers, workers...)
}

// Consume each received block from SDS Spaghetti broadcast
func (manager *Manager) categorize_current_smartcontracts() {
	current_logger := app_log.Child(manager.logger, "current")

	current_logger.Info("starting to consume subscribed blocks...")

	for {
		time.Sleep(time.Second * time.Duration(1))

		if len(manager.current_workers) == 0 || manager.subscribed_blocks.IsEmpty() {
			continue
		}

		// consume each block by workers
		for {
			block := manager.subscribed_blocks.Pop().(*spaghetti_block.Block)

			for _, worker := range manager.current_workers {
				if block.BlockNumber <= worker.smartcontract.CategorizedBlockNumber {
					continue
				}
				logs := block.GetForSmartcontract(worker.smartcontract.Address)
				categorized_logs, _ := worker.categorize(logs)

				current_logger.Info("categorized a smartcontract", "address", worker.smartcontract.Address, "logs amount", len(categorized_logs))

				smartcontracts := []*smartcontract.Smartcontract{worker.smartcontract}

				push := message.Request{
					Command: "",
					Parameters: map[string]interface{}{
						"smartcontracts": smartcontracts,
						"logs":           categorized_logs,
					},
				}
				request_string, _ := push.ToString()

				current_logger.Info("send a notification to SDS Categorizer")

				_, err := manager.pusher.SendMessage(request_string)
				if err != nil {
					current_logger.Fatal("sending notification to SDS Categorizer", "message", err)
				}
			}
		}
	}
}

// We start to consume the block information from SDS Spaghetti
// And put it in the queue.
// The worker will start to consume them one by one.
func (manager *Manager) subscribe() {
	sub_logger := app_log.Child(manager.logger, "subscriber")

	ctx, err := zmq.NewContext()
	if err != nil {
		sub_logger.Fatal("failed to create a zmq context", "message", err)
	}

	spaghetti_env, _ := service.New(service.SPAGHETTI, service.BROADCAST)
	subscriber, sockErr := ctx.NewSocket(zmq.SUB)
	if sockErr != nil {
		sub_logger.Fatal("failed to create a zmq sub socket", "message", sockErr)
	}

	plain, _ := argument.Exist(argument.PLAIN)

	if !plain {
		sub_logger.Info("setting up authentication key")
		categorizer_env, _ := service.New(service.CATEGORIZER, service.SUBSCRIBE)
		err := subscriber.ClientAuthCurve(spaghetti_env.BroadcastPublicKey, categorizer_env.BroadcastPublicKey, categorizer_env.BroadcastSecretKey)
		if err != nil {
			sub_logger.Fatal("failed to set up authentication key", "message", err)
		}
	}

	err = subscriber.Connect("tcp://" + spaghetti_env.BroadcastUrl())
	if err != nil {
		sub_logger.Fatal("failed to connect to blockchain client", "url", spaghetti_env.BroadcastUrl(), "message", err)
	}
	err = subscriber.SetSubscribe(manager.Network.Id + " ")
	if err != nil {
		sub_logger.Fatal("failed to set the subscribed topic string", "topic", manager.Network.Id+" ", "message", err)
	}

	sub_logger.Info("waiting for categorized data from blockchain")

	for {
		msgRaw, err := subscriber.RecvMessage(0)
		if err != nil {
			sub_logger.Fatal("receiving socket message", "message", err)
		}
		sub_logger.Info("received a message from client worker")

		broadcast, _ := message.ParseBroadcast(msgRaw)

		reply := broadcast.Reply

		block_number, _ := reply.Parameters.GetUint64("block_number")
		network_id, _ := reply.Parameters.GetString("network_id")
		if network_id != manager.Network.Id {
			sub_logger.Warn("skipping, since categorizer manager catched an event for another blockchain", "network_id", network_id, "manager_network_id", manager.Network.Id)
			continue
		}

		// Repeated subscriptions are not catched
		if manager.subscribed_earliest_block_number != 0 && block_number < manager.subscribed_earliest_block_number {
			continue
		} else if manager.subscribed_earliest_block_number == 0 {
			manager.subscribed_earliest_block_number = block_number
		}

		timestamp, _ := reply.Parameters.GetUint64("block_timestamp")

		raw_logs, _ := reply.Parameters.ToMap()["logs"].([]interface{})
		logs, _ := spaghetti_log.NewLogs(raw_logs)

		new_block := spaghetti_block.NewBlock(manager.Network.Id, block_number, timestamp, logs)

		sub_logger.Info("add a block to consume", "block_number", block_number, "event log amount", len(logs))
		manager.subscribed_blocks.Push(new_block)
	}
}
