## LinkEye

## Clone the source
    mkdir -p $GOPATH/src/github.com/linkeye/
    cd $GOPATH/src/github.com/linkeye/
    git clone https://github.com/linkeye/linkeye

## Building the source

    make linkeye

or, to build the full suite of utilities:

    make all



## Running Linkeye


```
$ linkeye
```
linkeye will connect to the main net, or use 

```
$ linkeye console
```
linkeye will connect to the main net and open an interactive console, we can use it to test linkeye function.
