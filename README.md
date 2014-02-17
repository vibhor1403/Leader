Leader
=========

Leader is a go language implementation of **Raft consensus protocol**. This protocol is mainly used for synchronization between different servers using message passing technique.

The algorithm implemented for synchronization is given in this paper:

[In Search of an Understandable Consensus Algorithm](https://ramcloud.stanford.edu/wiki/download/attachments/11370504/raft.pdf) given by Diego Ongaro and John Ousterhout

There are a lot more things to do as given in the paper, but the primary aim of this project is to make this library robust. Many test cases are included for the same.

As of now, **Leader election process** is implemented to its full extent.

Overview
----------

As a part of Raft consensus protocol, the project in its current state implements leader election process. This process can be summarized as follows.

Raft works as client-server system and all the activities are managed by the server itself. For that a leader is elected amongs all the peers, and once elected, it will be responsible for all synchronization related activites, unless something unusual happens.

Inititally all servers start as FOLLOWER. Raft ensures that ther is only one leader at any particular instant. For leader election process, server 
promote themselves as CANDIDATE and request for votes from its peers. Only after getting majority of votes (quorum size) can the candidate become LEADER.

The following figure specifies the whole server states and how they can move from one state to other.

![Alt text](https://f.cloud.github.com/assets/6353786/2181747/b53f24d4-9763-11e3-8cd3-3c56dc28a6f9.png)

TODO
-------------

Many things need to be added to it to make a complete Raft library. However, in the current implementation also, some things can be added to make it more robust.

1. Testing a scenario, where if a leader is chosen and suddenly that server is partitioned off from other. In such case, there can be more than one leader.
2. Saving the current term in disk, so that when the server wakes up, reads that value.
3. Heavy stress testing, where servers are going down and waking up very quickly. However, this scenario is highly unlikely in real environment.

Usage
--------------

To retrieve the repository from github, use: 
```sh
go get github.com/vibhor1403/Leader
```
To test the cluster library, use:
```sh
go test -v github.com/vibhor1403/Leader/Raft
```
This will test the library on all aspects, considering 5 servers passing messages between each other.

To run a single instance of server, use:
```sh
Leader -pid=<pid-of-this-server>
```

This pid should be present in the config.json file. This will start the server and broadcast a message to all its peers.

***Assumption*** : config.json is present in the same place where the bash terminal is, when this command is issued.


API's
-------

[![GoDoc](http://godoc.org/github.com/vibhor1403/Leader/Raft?status.png)](http://godoc.org/github.com/vibhor1403/Leader/Raft)

The following few functions can be used:

* `New(pid int, conf string)` - starts a new server with the given pid and location of configuration file.

* `State()` - gets the current state (LEADER, FOLLOWER, CANDIDATE) of the server.

* `Leader()` - In stable state, gets the leader pid.

* `UnsetPartitionValue()` - Unsets the value of partitionArray, which simulates network failure. 

* `ServerStopped()` - Channel which signifies that the server is completely closed. Simulates break down of the server.


Tweaking
-----------


Few constants are defined in the Raft.go file which sets the timeout duration and heartbeat interval. They can be changed according to the network situtaion. However majority of code testing is done with the following default values:

* `timeoutDuration = 200 miillisecond` - It determines the base duration after which follower starts a new election if no message is recieved from other servers. The actual duration is kept a bit random so that all servers don't start election at same time.

* `heartbeatinterval    = 50 millisecond` - Duration in which leader sends keep alive messages to its peers.

* `recieveTO = 2 second` - Timeout for listen socket. After this timeout the listen socket will get closed, if nothing is recieved on it during this time.

--------------------

For testing purpose also, the following constants are defined in Raft_test.go and can be altered accordingly:

* `toleranceDuration = 10 seconds` - Maximum tolerance in which if no leader is elected, test fails.

* `pingDuration		= 100 milliseconds` - After every pingDuration current leader is found out, and if more than one leader remains, test fails.

* `testDuration		= 20 seconds` - Total time for which test cases need to run.

JSON format
----------------
This file contains the pid and url of all servers in the cluster. It is required to give the value of total correct.
```sh
{"object": 
        {
           	"total": 2,
       		"Servers":
       		[
               		{
                       		"mypid": 1,
                       		"url": "127.0.0.1:100001"
                       	},
			{
                       		"mypid": 2,
                       		"url": "127.0.0.1:100002"
                       	}
       		]
    	}
}
```

How Raft works??
------------------------

The Raft package first initializes the data structure needed for that particular server. This data structure conatains the following main fields:

* **Mypid** - Contains the pid of this server.
* **Url** - Contains the url of this server.
* **Peers** - Contains a list of all peers to whom to connect to.
* **Input** - Input channel (for storing incoming messages).
* **Output** - Output channel (for storing outgoing messages).
* **Error** - Error channel for controlling closing of server.
* **Sockets** - Array of all outbound sockets.
* **Term** - Local counter, which is sent across peers for cordination.
              
The library then start three goroutines:

* One for sending messages to other servers.
* One for recieving messages
* One to implement the main loop for leader selection.

