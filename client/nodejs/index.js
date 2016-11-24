/**
 * WebSocket NodeJS Client
 */
var WebSocket = require('ws')
  , ws = new WebSocket('ws://localhost:3000/testnamespace');
ws.on('open', function() {
    ws.send('something');
});
ws.on('message', function(message) {
    console.log('received: %s', message);
});