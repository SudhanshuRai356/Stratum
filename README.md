## How to run this shit

make sure you have the go pkg with version >= 1.24.11

go to the folder after cloning this repo

run this exactly pwease:-

```go
go run ./cmd/stratum <your .torrent file>
```

it should start downloading the torrent in the main directory if the peers are healthy lol

to see the realtime update of the file you are downloading

run this command as is in a separate terminal :3

```
  watch -n 1 du -h <file name>
```