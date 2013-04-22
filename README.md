# gorem

go reverse proxy by mattn

## usage

    $ gorem  -c config.json

## setting

    
    {
        "entries": [
            {
                "path": "/foo", /* transform requests for /app/foo/ */
                "backend": "http://localhost:5003"
            },
            {
                "path": "/bar", /* transform requests for /app/foo/ */
                "backend": "http://localhost:5004"
            }
        ],
        "root": "/app/",
        "address": "127.0.0.1:5000" /* listen port 5000 */
    }
