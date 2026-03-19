# Building on z/OS

Building k6 with this extension on z/OS USS requires specific environment configuration.

## Environment Setup

```bash
export SSL_CERT_FILE=$ZOPEN_CA                    # CA certificates for HTTPS
export GOCACHE=/tmp/$USER/go-cache                # Use /tmp for build cache (more space)
export GOMODCACHE=/tmp/$USER/go-mod               # Use /tmp for module cache
export LIBPATH=/usr/lib:$LIBPATH                  # Include system libraries for linker
export GOMAXPROCS=2                               # Limit parallelism to avoid OOM
```

## Build Command

```bash
xk6 build \
  --with xk6-tn3270=/path/to/xk6-tn3270 \
  --replace github.com/spf13/afero=github.com/spf13/afero@v1.11.0 \
  --output ./k6
```

## Notes

1. **SSL certificates**: Set `SSL_CERT_FILE` to a valid CA bundle (e.g., `$ZOPEN_CA` from z/Open).
2. **Disk space**: Use `/tmp` for Go caches — home directories often have limited space.
3. **Linker libraries**: Add `/usr/lib` to `LIBPATH` for `iewbndd6.so`.
4. **afero replace**: The default afero v1.1.2 is incompatible with z/OS — use v1.11.0+.
5. **Memory**: Limit `GOMAXPROCS` if builds get killed (OOM).

## Full Build Script

```bash
#!/bin/bash
source ~/.profile

export SSL_CERT_FILE=$ZOPEN_CA
export GOCACHE=/tmp/$USER/go-cache
export GOMODCACHE=/tmp/$USER/go-mod
export LIBPATH=/usr/lib:$LIBPATH
export GOMAXPROCS=2

mkdir -p $GOCACHE $GOMODCACHE

xk6 build \
  --with xk6-tn3270=. \
  --replace github.com/spf13/afero=github.com/spf13/afero@v1.11.0 \
  --output ./k6
```
