//Package Raft implements the Leader election process of Raft consensus algorithm.
package Raft

import (
	"encoding/json"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"
)

// Various constants for configuring server...
const (
	NONE		= 0
	BROADCAST   = -1
	MAX         = 10
	FOLLOWER    = 1
	CANDIDATE   = 2
	LEADER      = 3
	CLOSEDSTATE	= 4
	REQUESTVOTE = 1
	HEARTBEAT   = 2
	GRANTVOTE   = 3
	MODIFY      = 4
	NOVOTE      = 5
	timeoutDuration	= 200 * time.Millisecond
	heartbeatinterval	= 50 * time.Millisecond
	recieveTO	= 2*time.Second
)

// Envelope describes the message structure to be followed to communicate with other servers.
type Envelope struct {
	//SendTo specifies the pid of the recieving system. Setting it to -1, will broadcast the message to all peers.
	SendTo int

	// SendBy specifies the pid of the sender.
	SendBy int

	// MsgId is an id that globally and uniquely identifies the message, meant for duplicate detection at
	// higher levels. It is opaque to this package.
	MsgId int64

	//Msg comprises of the actual message.
	Msg interface{}

	//Term contains the current local value of term. Servers synchronize with each other using this value, by message passing.
	Term int
	
	// 	Type can hold the following values:
	//	REQUESTVOTE = 1
	//	HEARTBEAT   = 2
	//	GRANTVOTE   = 3
	//	MODIFY      = 4
	//	NOVOTE      = 5
	Type int

	// VoteTo is used for synchronizing in case any votes come out of order..
	VoteTo int
}

// Server interface provides various methods for retriving information about the cluster.
type Server interface {
	// Pid is the Id of this server
	Pid() int

	// Peers contains array of other servers' ids in the same cluster
	Peers() []int

	// Outbox is the channel to use to send messages to other peers.
	Outbox() chan *Envelope

	// Inbox is the channel to receive messages from other peers.
	Inbox() chan *Envelope
	
	// Leader tells the present leader.
	Leader() int
	
	// Error channel helps to control the closing of servers. It shuts down all the sockets and channels.
	Error() chan bool
	
	// State returns the current state of the server. It can have following values:
	//	FOLLOWER    = 1
	//	CANDIDATE   = 2
	//	LEADER      = 3
	//	CLOSEDSTATE	= 4
	State() int

	// Server Stopped channel synchronizes the closing of server. It will have a value, only when this server is completely closed,	
	// ie all the sockets, channels and goroutines are properly shut down.
	ServerStopped() chan bool
	
	// Length of this array will be total+1
	// Stores whether the pid = index is connected to the current machine or not.. 
	// Used for inducing faults, during testing
	// For each peer holds a boolean value whether to connect to that or not.
	// Helps in simulating network faults and partitions.	
	PartitionArray() []bool
	
	// Makes an outgoing link to particular peer.
	SetPartitionValue(index int)
	
	// Breakes an outgoing link with particular peer.
	UnsetPartitionValue(index int)

}

type jsonobject struct {
	Object ObjectType
}

type ObjectType struct {
	Total   int
	Servers []ServerConfig
}

//ServerConfig is structure containing all the information needed about this server
type ServerConfig struct {
	// Pid of this server
	Mypid 				int
	// Url (ip:port) of this server
	Url 				string
	// Input channel for holding incoming data
	Input 				chan *Envelope
	// Output channel for sending data to other peers
	Output       		chan *Envelope
	// ErrorChannel is for synchronizing server shutdown.
	ErrorChannel 		chan bool
	// Stopped channel indicates proper closing of server.
	Stopped				chan bool
	// Stopping channel indicates the process of server shutdown.
	Stopping			chan bool
	// Array of peers
	Mypeers 			[]int
	// Array of all sockets opened by this server (contains 4 outbound sockets)
	Sockets 			[]*zmq.Socket
	// mutex for synchronizing and getting read and write locks.
	mutex     			sync.RWMutex
	// state has values: 1 - follower, 2 - candidate, 3 - leader, 4 - closed
	state 				int
	// stores pid, can have value 0, indicating no vote, as pid's start from 1
	votedFor 			int
	// stores the current local value of term.
	term     			int
	// stores the pid of leader. At stable state, all servers will have same value in this field.
	leader          	int
	// At what point to break the election process in idle situation.
	electionTODuration 	time.Duration
	// Rate of sending keep alive messages.
	heartbeatDuration 	time.Duration
	// majority holds the maximum fault tolerance capability.
	majority          	int
	// peerPartition contains the array with information of network connectivity.
	peerPartition		[]bool
}

func (sc *ServerConfig) Pid() int {
	return sc.Mypid
}

func (sc *ServerConfig) Peers() []int {
	return sc.Mypeers
}

func (sc *ServerConfig) Inbox() chan *Envelope {
	return sc.Input
}

func (sc *ServerConfig) Outbox() chan *Envelope {
	return sc.Output
}

func (sc *ServerConfig) Error() chan bool {
	return sc.ErrorChannel
}

func (sc *ServerConfig) Leader() int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.leader
}

func (sc *ServerConfig) State() int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.state
}

func (sc *ServerConfig) ServerStopped() chan bool {
	return sc.Stopped
}

func (sc *ServerConfig) SetPartitionValue(index int) {
	sc.peerPartition[index+1] = true 
}

func (sc *ServerConfig) UnsetPartitionValue(index int) {
	sc.peerPartition[index+1] = false
}

func (sc *ServerConfig) PartitionArray() []bool {
	return sc.peerPartition
}

// mapping maps pid of the server to its url
var mapping map[int]string

// New function is the main function, which initializes all the parameters needed for server to function correctly. Further, it also
// starts routines to check for the channels concurrently..
func New(pid int, conf string) Server {

	file, e := ioutil.ReadFile(conf)
	if e != nil {
		panic("Could not read file")
	}
	var jsontype jsonobject
	err := json.Unmarshal(file, &jsontype)
	if err != nil {
		panic("Wrong format of conf file")
	}

	// Inialization of mapping and server parameters.
	mapping = make(map[int]string)
	sc := &ServerConfig{
		Mypid:             	pid,
		Url:               	mapping[pid],
		Input:             	make(chan *Envelope),
		Output:            	make(chan *Envelope),
		ErrorChannel:      	make(chan bool),
		Stopped:			make(chan bool),
		Stopping:			make(chan bool),
		Mypeers:           	make([]int, jsontype.Object.Total-1),
		Sockets:           	make([]*zmq.Socket, jsontype.Object.Total-1),
		state:             	FOLLOWER,
		votedFor:          	NONE,
		term:              	0,
		leader:            	NONE,
		electionTODuration:	timeoutDuration,
		heartbeatDuration: 	heartbeatinterval,
		majority:          	(jsontype.Object.Total/2 + 1),
		peerPartition:		make([]bool, jsontype.Object.Total+1) }
		
		
	for i:=0 ; i< jsontype.Object.Total+1; i++ {
		sc.peerPartition[i] = true
	} 		
		

	// Populates the peers of the servers, and opens the outbound ZMQ sockets for each of them.
	k := 0
	for i := 0; i < jsontype.Object.Total; i++ {
		mapping[jsontype.Object.Servers[i].Mypid] = jsontype.Object.Servers[i].Url

		if jsontype.Object.Servers[i].Mypid != pid {
			sc.Mypeers[k] = jsontype.Object.Servers[i].Mypid
			sc.Sockets[k], err = zmq.NewSocket(zmq.DEALER)

			if err != nil {
				panic(fmt.Sprintf("Unable to open socket for %v as %v", sc.Mypeers[k], err))
			}
			err = sc.Sockets[k].Connect("tcp://" + mapping[sc.Mypeers[k]])
			if err != nil {
				panic(fmt.Sprintf("Unable to connect socket for %v as %v", sc.Mypeers[k], err))
			}
			k++
		}
	}
	sc.Url = mapping[pid]

	// 	Starts two go routines each for input and output channel functionalities..
	// and a main Loop, checks for state forever. Only exits when server shuts down, or closed state is reached.
	
	go CheckOutput(sc)
	go Listen(sc)
	go CheckState(sc)
	return sc
}

// CheckState checks constantly for the current state of the server, and accordingly calls the appropriate loop.
// In closed state, it waits for all the go routines to properly shut-down and then signals the public channel.
func CheckState(sc *ServerConfig) {

	for {
		sc.mutex.Lock()
		state := sc.state
		sc.mutex.Unlock()
		//fmt.Println("state", sc.state)

		switch state {
		case FOLLOWER:
			followerLoop(sc)
		case CANDIDATE:
			candidateLoop(sc)
		case LEADER:
			leaderLoop(sc)
		case CLOSEDSTATE:
			<- sc.Stopping
			<- sc.Stopping
			//fmt.Println(sc.Mypid, "stopped")
			sc.Stopped <- true
			return
		}
	}
}


// Gets a write lock and sets the value of the current leader.
func setLeader(sc *ServerConfig, pid int) {
	sc.mutex.Lock()
	sc.leader = pid
	sc.mutex.Unlock()
}
// Gets a read lock and reads the value of the current leader.
func getLeader(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.leader
}
// Gets a write lock and sets the value of the current term.
func setTerm(sc *ServerConfig, term int) {
	sc.mutex.Lock()
	sc.term = term
	sc.mutex.Unlock()
}
// Gets a write lock and sets the value of the peer to whom the vote has been given.
func setVotedFor(sc *ServerConfig, pid int) {
	sc.mutex.Lock()
	sc.votedFor = pid
	sc.mutex.Unlock()
}
// Gets a read lock and reads the value of the current term.
func getTerm(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.term
}
// Gets a read lock and reads the value of the peer to whom this server voted..
func getVotedFor(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.votedFor
}
// Gets a read lock and reads the value of the current state.
func getState(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.state
}
// Gets a write lock and sets the value of the current state.
func setState(sc *ServerConfig, state int) {
	sc.mutex.Lock()
	sc.state = state
	sc.mutex.Unlock()
}

// Waits for a random time between given duration and duration*2, and sends the current time on
// the returned channel.
func RandomTimer(duration time.Duration) <-chan time.Time {
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	d, delta := duration, duration
	if delta > 0 {
		d += time.Duration(rand.Int63n(int64(delta)))
	}
	return time.After(d)
}

// follower loop realizes the logic of FOLLOWER as given in Raft consensus algorithm.
func followerLoop(sc *ServerConfig) {

	timeChan := RandomTimer(sc.electionTODuration)

	//fmt.Println(sc.Mypid, "state", state)
	for getState(sc) == FOLLOWER {
		//wait for inbox channel
		//fmt.Println(sc.Mypid, "waiting at channel", sc.term, sc.state)
		select {
		case envelope := <-sc.Input:
			//fmt.Println("recieve, infollower", sc.Mypid, envelope)
			// if a lower term message is recieved, send a modify message...
			if envelope.Term < getTerm(sc) {
				//fmt.Println(sc.Mypid, "send, infollower1", envelope.SendBy, sc.term, "MODIFY")
				sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if heartbeat recieved, reset timer, update term if needed
			} else if envelope.Term > getTerm(sc) && envelope.Type == REQUESTVOTE {
				setTerm(sc, envelope.Term)
				setVotedFor(sc, envelope.SendBy)
				//fmt.Println(sc.Mypid, "send, infollower2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
				sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				// If heartbeat is recieved, sets the current term and updates leader.
			} else if envelope.Type == HEARTBEAT {
				setTerm(sc, envelope.Term)
				setLeader(sc, envelope.SendBy)
				// if request for vote recieved, take decision of granting vote or not and sent vote on channel and reset timer.
			} else if envelope.Type == REQUESTVOTE {
				setTerm(sc, envelope.Term)
				if getVotedFor(sc) == NONE {
					setVotedFor(sc, envelope.SendBy)
					//fmt.Println(sc.Mypid, "send, infollower3", envelope.SendBy, getTerm(sc), "GRANTVOTE")
					sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				} else {
					//fmt.Println(sc.Mypid, "send, infollower4", envelope.SendBy, getTerm(sc), "NOVOTE")
					sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: NOVOTE, VoteTo: sc.votedFor}
				}
			}
			// reset the timer.
			timeChan = RandomTimer(sc.electionTODuration)
			// If timer expires, promote itself to candidate and start a new election.
		case <-timeChan :
			sc.mutex.Lock()
			sc.state = CANDIDATE
			sc.votedFor = NONE
			sc.mutex.Unlock()
			//fmt.Println(sc.Mypid, "become candidate", sc.term, sc.state)
			// If error channel has recieved a value, close the server.
		case <- sc.ErrorChannel :
			closeServer(sc)
		}
	}

}

// closeServer Stops the server and is called whenever a value id recieved on error channel. It simulates the breakdown of the server.
// closes all the channels and set the state to CLOSEDSTATE.
func closeServer(sc *ServerConfig) {
	setState(sc, CLOSEDSTATE)
	close(sc.Output)
	close(sc.ErrorChannel)
	close(sc.Input)
}
// candidateLoop realizes the logic of CANDIDATE as given in Raft consensus algorithm.
func candidateLoop(sc *ServerConfig) {
//	fmt.Println(sc.Mypid, "entered candidate")
	// increment term
	sc.mutex.Lock()
	sc.term++
	sc.mutex.Unlock()

//	fmt.Println(sc.Mypid, "term", sc.term)
	// vote for self
	setVotedFor(sc, sc.Mypid)
	totalVotesRecieved := 1
	//reset timer
	timeChan := RandomTimer(sc.electionTODuration)
	// send request for voting on all peers
	//fmt.Println("send, incandidate", -1, sc.term)
	sc.Output <- &Envelope{SendTo: -1, SendBy: sc.Mypid, Term: getTerm(sc), Type: REQUESTVOTE}

	for getState(sc) == CANDIDATE {

		select {
		// wait for inbox channel
		case envelope := <-sc.Input:
			//fmt.Println("recieve incandidate", sc.Mypid, envelope)
			// if a lower term message is recieved, send a modify message...
			if envelope.Term < getTerm(sc) {
				//fmt.Println(sc.Mypid, "send, incandidate1", envelope.SendBy, getTerm(sc), "MODIFY")
				sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if recieved message term is greater than my term, end election, become follower, return
			} else if envelope.Type == HEARTBEAT {
				setTerm(sc, envelope.Term)
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				setLeader(sc, envelope.SendBy)
				//fmt.Println(sc.Mypid, "became follower")
				return
			} else if envelope.Term > getTerm(sc) {
				setTerm(sc, envelope.Term)
				// if bigger term peer requests for vote, give him vote without thinking, as your vote to yourself is stale
				if envelope.Type == REQUESTVOTE {
					setVotedFor(sc, envelope.SendBy)
					//fmt.Println(sc.Mypid, "send, incandidate2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
					sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				}
				setState(sc, FOLLOWER)
				return
				// if vote is granted, add to total votes
			} else if envelope.Type == GRANTVOTE && envelope.VoteTo == sc.Mypid {
				totalVotesRecieved++
//				fmt.Println("Votes", totalVotesRecieved)
			}
			
			// if majority votes recieved, become leader, return
			if totalVotesRecieved >= sc.majority {
				setState(sc, LEADER)
				setVotedFor(sc, NONE)
				setLeader(sc, sc.Mypid)
				//fmt.Println(sc.Mypid, "became leader")
				return
			}
			// if timeout
		case <-timeChan:
			// become follower, end election.
			//fmt.Println(sc.Mypid, "became follower again")
			setVotedFor(sc, NONE)
			setState(sc, FOLLOWER)
			// If error channel has some value.
		case <- sc.ErrorChannel :
			closeServer(sc)
			
		}

	}

}
// leaderLoop realizes the logic of LEADER as given in Raft consensus algorithm.
func leaderLoop(sc *ServerConfig) {

	// start heartbeat timer
	// send message to all peers about the aliveness
	sc.Output <- &Envelope{SendTo: -1, SendBy: sc.Mypid, Term: getTerm(sc), Type: HEARTBEAT}
	heartTimeChan := time.NewTimer(sc.heartbeatDuration)

	for getState(sc) == LEADER {
		select {
		case <-heartTimeChan.C:
			fmt.Println(sc.Mypid, "sending heartbeat", -1, getTerm(sc))
			// send message to all peers about the aliveness
			sc.Output <- &Envelope{SendTo: -1, SendBy: sc.Mypid, Term: getTerm(sc), Type: HEARTBEAT}
			heartTimeChan = time.NewTimer(sc.heartbeatDuration)

		// wait for input
		case envelope := <-sc.Input:
			//fmt.Println("recieve inleader", sc.Mypid, envelope)

			// if a lower term message is recieved, send a modify message...
			if envelope.Term < getTerm(sc) {
				//fmt.Println(sc.Mypid, "send, inleader1", envelope.SendBy, getTerm(sc), "MODIFY")
				sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if recieved message term is greater than my term, end election, become follower, return
			} else if envelope.Type == HEARTBEAT {
				setTerm(sc, envelope.Term)
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				setLeader(sc, envelope.SendBy)
//				fmt.Println(sc.Mypid, "became follower")
				return
			} else if envelope.Term > getTerm(sc) {
				setTerm(sc, envelope.Term)
				// if message recieved with higher term from leader for vote request, demote itself as follower and grant vote.
				if envelope.Type == REQUESTVOTE {
					setVotedFor(sc, envelope.SendBy)
					//fmt.Println(sc.Mypid, "send, inleader2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
					sc.Output <- &Envelope{SendTo: envelope.SendBy, SendBy: sc.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				}
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				//fmt.Println(sc.Mypid, "became follower again")
				return
			}
		// If error.	
		case <- sc.ErrorChannel :
			closeServer(sc)
		}
	}

}

// CheckInput waits on input channel of the server and prints the data on standard output.
func CheckInput(sc *ServerConfig) {
	for {
		envelope, ok := <-sc.Input
		if !ok {
			panic("channels closed..")
		}
		fmt.Printf("Received msg from %d to %d\n", envelope.SendBy, envelope.SendTo)
	}
}

// CheckOutput waits on output channel, and according to the type of message recieved on this channel sends the data to other peers.
// While sending the data, it checks whether the network link is down or not, as simulated in the peerPartition array.
func CheckOutput(sc *ServerConfig) {
	for {
		x, ok := <-sc.Output
		// If Output channel is not closed.
		if ok {
			// If BROADCAST message
			if x.SendTo == -1 {
				for i := 0; i < len(sc.Mypeers); i++ {
					b, _ := json.Marshal(*x)
					if sc.peerPartition[sc.Mypeers[i]] {
						_, err := sc.Sockets[i].Send(string(b), 0)
						if err != nil {
							panic(fmt.Sprintf("Could not send message,%v,%v..%v", sc.Mypid, sc.Mypeers[i], err))
						}
					} else {
						//fmt.Println(sc.Mypid, "stopped message to ", sc.Mypeers[i])
						continue
					}
					
				}
			} else {
				b, _ := json.Marshal(*x)
				if sc.peerPartition[sc.Mypeers[findPid(sc, x.SendTo)]] {
					_, err := sc.Sockets[findPid(sc, x.SendTo)].Send(string(b), 0)
					if err != nil {
						panic(fmt.Sprintf("Could not send 1message,%v,%v..%v", sc.Mypid, sc.Mypeers[findPid(sc, x.SendTo)], err))
					}
				} else {
					//fmt.Println(sc.Mypid, "stopped message to ", sc.Mypeers[findPid(sc, x.SendTo)])
					continue
				}
			}
			// If output channel is closed, then it closes all the outbound sockets of the current server.
		} else {
			for i := 0; i < len(sc.Mypeers); i++ {
				sc.Sockets[i].Close()
			}
			sc.Stopping <- true
			return
		}
	}
}

// findPid returns the index of server's peers array which correspond to the given pid. If not found, returns -1
func findPid(sc *ServerConfig, pid int) int {
	for i := 0; i < len(sc.Mypeers); i++ {
		if pid == sc.Mypeers[i] {
			return i
		}
	}
	return -1
}

// Listen method waits on recieving socket to gather data from the peers. Wait timeout is set to 2 seconds. If no data is available for
// more than 2 seconds, listen socket is closed, and correspondingly the input channel is also closed, which enables another routine
// waiting on input channel to be notified.
// This timeout can be set to -1 if we want that socket remains open for indefinite amount of time.
func Listen(sc *ServerConfig) {
	listenSocket, er := zmq.NewSocket(zmq.DEALER)
	if er != nil {
		panic("Unable to open socket for listening")
	}
	defer listenSocket.Close()
	listenSocket.Bind("tcp://" + sc.Url)
	listenSocket.SetRcvtimeo(recieveTO)
	//listenSocket.SetRcvtimeo(-1)
	for {
		if getState(sc) == CLOSEDSTATE {
			sc.Stopping <- true
			return
		}
		msg, err := listenSocket.Recv(0)
		if err != nil {
			// If timeout happens and server is not issued a closed request, either the link is temporarily down, or all other peers are down.
			// In such case, start listening again.
			if getState(sc) != CLOSEDSTATE {
				go Listen(sc)
			} else {
				//close(sc.Input)
				sc.Stopping <- true
			}
			return
		}

		message := new(Envelope)
		json.Unmarshal([]byte(msg), message)
		if getState(sc) == CLOSEDSTATE {
			sc.Stopping <- true
			return
		} else {
			sc.Input <- message
		}
	}
}
