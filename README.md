# CometBFT DB migration tool

## Usage

Before running the migration tool, make sure to stop the CometBFT node and do a
backup of `~/.cometbft/data` folder.

```sh
go build
./db_migrate
```

This will migrate the goleveldb databases located at `~/.cometbft/data` to
pebbledb.

NOTE: Afterwards, please don't forget to change the `db_backend` configuration
in the `config.toml` file to `pebbledb`.

### Build flags

If the source database requires a build flag (cleveldb, boltdb, rocksdb,
badgerdb), you can specify it as follows:

```sh
go build -tags cleveldb
./db_migrate
```
