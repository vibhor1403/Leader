Raft
=========

Raft is a clustering library, which connect various servers with each other by exchanging messages between them (including broadcast messages). It takes the help of open source **zmq4** library for creating sockets and sending and recieving data among each other. 

The overall design goal of this project is to make this library as robust as possible. I have included many test cases for testing the same.

Usage
--------------
To retrieve the repository from github, use: 
```sh
go get github.com/vibhor1403/Raft
```
To test the cluster library, use:
```sh
go test -v github.com/vibhor1403/Raft/cluster
```
This will test the library on all aspects, considering 5 servers passing messages between each other.

To run a single instance of server, use:
```sh
Raft -pid=<pid-of-this-server>
```

This pid should be present in the config.json file. This will start the server and broadcast a message to all its peers.

***Assumption*** : config.json is present in the same place where the bash terminal is, when this command is issued.

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

How the cluster works??
------------------------

The cluster package first initializes the data structure needed for that particular server. This data structure conatains the following main fields:
* **Mypid** - Contains the pid of this server.
* **Url** - Contains the url of this server.
* **Peers** - Contains a list of all peers to whom to connect to.
* **Input** - Input channel (for storing incoming messages).
* **Output** - Output channel (for storing outgoing messages).
* **Sockets** - Array of all outbound sockets.
              
This library then initiates two goroutines, which sits on output and input channel respectively, and send the data to, or recieve data from the peers. They also check for closure of channel which helps them to notify other goroutines of the completion events.

***Note*** : Timeout for listen socket is set to 2 seconds. After this timeout, listen socket will close the input channel and also closes itself. This setting works fine in my system. However to change the timeout, we need to change the value in _cluster.go_ file manually.
