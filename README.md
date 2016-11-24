# WebSocket Server with Golang

![img](https://github.com/Samurais/websocket-go/blob/master/docs/assets/Screen Shot 2016-11-24 at 14.31.48.png)
## Server
Install go v1.7+
Create go path. Such as 

```
export GOPATH=~/go
export PATH=$PATH:$GOPATH/bin
```

### Install deps
```
go get github.com/valyala/bytebufferpool
go get github.com/gorilla/websocket 
```


### Start Sample Server
```
go run example/main.go
```

## Client

### Node.js Client
```
cd client/nodejs
npm install
node index.js
```

### Go Client
```
go run client.go
```