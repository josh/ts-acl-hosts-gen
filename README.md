# ts-acl-hosts-gen

Updates a [Tailscale ACL policy](https://tailscale.com/kb/1018/acls) [hosts section](https://tailscale.com/kb/1337/acl-syntax#hosts) with all the nodes on your network. This is best used in a small homelab Tailnet. Keep in mind nodes themselves may manipulate their own hostname and may be able to escalate pervileges by doing so.

## Installation

```sh
go get -tool github.com/josh/ts-acl-hosts-gen@latest
go tool github.com/josh/ts-acl-hosts-gen
```

## Usage

```sh
$ ts-acl-hosts-gen --help
usage: ts-acl-hosts-gen [flags] policy.hujson
  -api-key string
        Tailscale API key
  -oauth-id string
        Tailscale OAuth client ID
  -oauth-secret string
        Tailscale OAuth client secret
```
