# Dockpin

Install dockpin with: `go install github.com/Jille/dockpin@latest`

Dockpin helps you achieve repeatable builds. It pins base images in your Dockerfile, and packages you install with apt-get.

`dockpin docker pin -f Dockerfile` rewrites your Dockerfile to use the latest digest for each image. Docker will then use exactly that image until you upgrade it.

Dockpin can also pin apt packages, though it's slightly more complex:

```shell
$ (echo postgresql-12; echo curl) > dockpin-apt.pkgs
$ dockpin apt pin
```

then you can change your `apt-get update && apt-get install -y postgresql-12 curl && apt-get clean && rm -rf /var/lib/apt/lists/*` in your Dockerfile to:

```
FROM ghcr.io/jille/dockpin AS dockpin
FROM ubuntu:focal
COPY --from=dockpin /bin/dockpin /usr/local/sbin/dockpin
COPY dockpin-apt.lock /tmp
RUN /usr/local/sbin/dockpin apt install -p /tmp/dockpin-apt.lock
[...]
```

## Why repeatable builds?

If you do a small cherrypick, to fix a bug, and you're going to roll that out to prod with an accellerated push, you don't want to accidentally also pick up a new Python version.

Increasingly more people do pin versions, but never upgrade and stay on that version forever. That makes security folks shudder.

Dockpin aims to make it easy to move to new versions *when you want*.

## Docker pinning

This is pretty easy. You can either make dockpin rewrite your Dockerfile in place:

```shell
$ dockpin docker pin [-f your.Dockerfile]
```

or control output yourself:

```shell
$ dockpin docker pin -f - < Dockerfile.template > Dockerfile
```

## Apt pinning

You should create a file called dockpin-apt.pkgs which contains one Debian/Ubuntu package per line. After that you can run `dockpin apt pin` which generates dockpin-apt.lock, which contains the URLs and size/hash of each .deb file to use.

When you run `dockpin apt install` in your Dockerfile, it will read (only) dockpin-apt.lock and install all the listed packages at the pinned versions.

The easiest way to get the `dockpin` binary in your Docker build is by grabbing it from the ghcr.io/jille/dockpin image (as shown in the example at the top of this README).

Note that the Debian/Ubuntu archives will eventually delete the old package you pinned from their mirrors. At that point you'll get an error when you try to build (rather than a silent upgrade). You can reproduce your build by somehow finding the old .deb file and changing the lock file to point at whichever URL you put it at. You can also COPY it into /var/cache/apt/archives/ and `dockpin apt install` will use that without downloading.

We rely on apt(8) to figure out which dependencies you already have / need to install. However, that does mean that we need to do the pinning on the same base image as you'll run `dockpin apt install`. We try to guess this automatically by parsing your Dockerfile, but that might fail and you'll need to pass `--base-image=ubuntu:focal` (or whatever image you use).
