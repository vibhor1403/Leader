package Raft_test


/*
All the testing is done keeping json file with 5 server configuration and pid's from 1-5
Below is the json sample file, between the lines containing *

**********************************************************************************************
{"object": 
    	{
       		"total": 5,
       		"Servers":
       		[
               		{
                       		"mypid": 1,
                       		"url": "127.0.0.1:100001"
                       	},
			{
                       		"mypid": 2,
                       		"url": "127.0.0.1:100002"
                       	},
			{
                       		"mypid": 3,
                       		"url": "127.0.0.1:100003"
                       	},
			{
                       		"mypid": 4,
                       		"url": "127.0.0.1:100004"
                       	},
			{
                       		"mypid": 5,
                       		"url": "127.0.0.1:100005"
                       	}
       		]
    	}
}
****************************************************************************************
*/

import (
	"github.com/vibhor1403/Leader/Raft"
	"testing"
	"fmt"
	"time"
	"sync"
	"math/rand"
)

// Following constants can be changed in order to do performance testing.
const (
	toleranceDuration 	= 5 * time.Second
	pingDuration		= 100 * time.Millisecond
	testDuration		= 10 * time.Second
	MAXTOLERANCE		= 2
)


// testUnanimousLeadership will check for number of leaders currently accross all peers. 
// Will break if no leader for long time, or more than one leader at any time.
func testUnanimousLeadership(server [5]Raft.Server, total int, wg *sync.WaitGroup) {

	idleLimit 	:= time.NewTimer(toleranceDuration)
	checkLeader := time.NewTimer(pingDuration)
	closeMe		:= time.NewTimer(testDuration)
	
	
	for {
		select{	
		case <- checkLeader.C :
			count := 0 
			for i := 0; i < total; i++ {
				//dbg.Println(i, server[i].State())
				if server[i].State() == Raft.LEADER {
					dbg.Println("leader", i+1)
					count++
				}
			}
			if count > 1 {
				panic ("More than one leader at a time")	
			} else if count == 1 {
				idleLimit 	= time.NewTimer(toleranceDuration)
			}
			checkLeader = time.NewTimer(pingDuration)
		case <- idleLimit.C :
			panic ("No leader for so long")
		case <- closeMe.C :
			dbg.Println("In here")
			wg.Done()
			return
		}
	}
}

// breakServer take index of server and disconnects the server with all other peers. Simulates link failures.
func breakServer(server [5]Raft.Server, index int, total int) {
	for i:=0; i<total; i++ {
		if i != index {
			server[index].UnsetPartitionValue(i)
			//server[i].UnsetPartitionValue(index)
		}
	}
}
// makeServer, take index of server and reconnects to  all other peers. Start sending message to them.
func makeServer(server [5]Raft.Server, index int, total int) {
	for i:=0; i<total; i++ {
		if i != index {
			server[index].SetPartitionValue(i)
			server[i].SetPartitionValue(index)
		}
	}
}
// breakLink take two indices of server and breaks link between them.
func breakLink(server [5]Raft.Server, first int, second int) {
	server[first].UnsetPartitionValue(second)
	server[second].UnsetPartitionValue(first)
}
// take index of server...
func makeLink(server [5]Raft.Server, first int, second int) {
	server[first].SetPartitionValue(second)
	server[second].SetPartitionValue(first)
}

type Debug bool
//// For debugging capabilities..
func (d Debug) Println(a ...interface{}) {
	if d {
		fmt.Println(a...)
	}
}
const dbg Debug = false

//No fault testing (IDEAL test)
func Test_NoFault(t *testing.T) {


	total := 5
	var server [5]Raft.Server
	for i := 0; i < total; i++ {
		server[i] = Raft.New(i+1, "../config.json")
		dbg.Println("opening ", i+1)
	}

	wg 			:= new(sync.WaitGroup)
	wg.Add(1)
	
	go testUnanimousLeadership(server, total, wg)
	
	wg.Wait()

	for i:=0; i<total; i++ {
		dbg.Println("closing", i+1)
		server[i].Error() <- true
		<- server[i].ServerStopped()
		dbg.Println("closed", i+1)
	}
	t.Log("No faults test passed.")

}

// Inducing random faults, less than the threshold capacity.
func Test_RandomFault(t *testing.T) {

	testChannel1 := time.NewTimer(testDuration)
	testChannel2 := time.NewTimer(testDuration)
	total := 5
	var server [5]Raft.Server
	for i := 0; i < total; i++ {
		server[i] = Raft.New(i+1, "../config.json")
		dbg.Println("opening ", i+1)
	}
	
	//can't be greater than 2
	serversClosed := 0
	
	induceFault		:= Raft.RandomTimer (time.Second)
	go func() {
		for {
			select {
			case <- induceFault :
				rand := rand.New(rand.NewSource(time.Now().UnixNano()))
				index := rand.Intn(total)
				if server[index].State() != Raft.CLOSEDSTATE && serversClosed < MAXTOLERANCE { 
					dbg.Println("closing" , index+1)
					server[index].Error() <- true
					<- server[index].ServerStopped()
					dbg.Println("closed" , index+1)
					serversClosed++
					dbg.Println(serversClosed)
				}
				induceFault		= Raft.RandomTimer (time.Second)
			case <- testChannel1.C :
				return
			}
		}
	}()
	
	removeFault		:= Raft.RandomTimer (time.Second)
	go func() {
		for {
			select {
			case <- removeFault :
				rand := rand.New(rand.NewSource(time.Now().UnixNano()))
				index := rand.Intn(total)
				if server[index].State() == Raft.CLOSEDSTATE { 
					dbg.Println("opening" , index+1)
					server[index] = Raft.New(index+1, "../config.json")
					serversClosed--
					dbg.Println("opened" , index+1)
					//dbg.Println(serversClosed)
				}
				removeFault		= Raft.RandomTimer (time.Second)
			case <- testChannel2.C :
				return
			}
		}
	}()
	
	wg 			:= new(sync.WaitGroup)
	wg.Add(1)
	
	go testUnanimousLeadership(server, total, wg)
	wg.Wait()
	dbg.Println("yahan2")
	for i:=0; i<total; i++ {
		dbg.Println(i, server[i].State())
		if server[i].State() != Raft.CLOSEDSTATE {
			dbg.Println("closing", i+1)
			server[i].Error() <- true
			<- server[i].ServerStopped()
			dbg.Println("closed", i+1)
		}
	}
	t.Log("Random faults test passed.")

}

//Minority failure test.. Stopped maximum possible servers.
func Test_KnownFault(t *testing.T) {

	testChannel1 := time.NewTimer(testDuration / 2)
	total := 5
	var server [5]Raft.Server
	for i := 0; i < total; i++ {
		server[i] = Raft.New(i+1, "../config.json")
		dbg.Println("opening ", i+1)
	}
	
	//can't be greater than 2
	serversClosed := 0
	
	induceFault		:= Raft.RandomTimer (time.Second)
	go func() {
		for {
			select {
			case <- induceFault :
				pid := 0
				for i:=0; i<total; i++ {
					if server[i].State() != Raft.CLOSEDSTATE {
						pid = server[i].Leader()
						break
					}
				}
				if pid != 0 && serversClosed < MAXTOLERANCE {
					dbg.Println("closing", pid)
					server[pid-1].Error() <- true
					<- server[pid-1].ServerStopped()
					serversClosed++
					dbg.Println("closed", pid)
				}
				induceFault		= Raft.RandomTimer (time.Second)
			case <- testChannel1.C :
				return
			}
		}
	}()
		
	wg 			:= new(sync.WaitGroup)
	wg.Add(1)
	
	go testUnanimousLeadership(server, total, wg)
	wg.Wait()
	dbg.Println("yahan2")
	for i:=0; i<total; i++ {
		dbg.Println(i, server[i].State())
		if server[i].State() != Raft.CLOSEDSTATE {
			dbg.Println("closing", i+1)
			server[i].Error() <- true
			<- server[i].ServerStopped()
			dbg.Println("closed", i+1)
		}
	}
	t.Log("Maximum break test passed.")

}

// Test for partitioning the network, simulating network failures.
func Test_Partitioning(t *testing.T) {

	testChannel1 := time.NewTimer(testDuration)
	total := 5
	var server [5]Raft.Server
	for i := 0; i < total; i++ {
		server[i] = Raft.New(i+1, "../config.json")
		dbg.Println("opening ", i+1)
	}
	
	induceFault		:= Raft.RandomTimer (time.Second)
	go func() {
		localCount := 0
		for {
			select {
			case <- induceFault :
				switch localCount {
				case 0:
					breakServer(server, 0, total)
				case 1:
					breakServer(server, 1, total)
				case 5:
					breakServer(server, 2, total)
				case 6:
					makeServer(server, 2, total)
				case 7:
					makeServer(server, 0, total)
				case 8:
					makeServer(server, 1, total)
				}
				localCount++
				induceFault		= Raft.RandomTimer (time.Second)
			case <- testChannel1.C :
				return
			}
		}
	}()
	
	wg 			:= new(sync.WaitGroup)
	wg.Add(1)
	
	go 	testUnanimousLeadership(server, total, wg)
	
	wg.Wait()

	for i:=0; i<total; i++ {
		dbg.Println("closing", i+1)
		server[i].Error() <- true
		<- server[i].ServerStopped()
		dbg.Println("closed", i+1)
	}
	t.Log("Partitioning test passed.")

}
