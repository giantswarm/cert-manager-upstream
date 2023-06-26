# cert-manager-upstream

This repository is a fork of [https://github.com/cert-manager/cert-manager][upstream-repo].

## Branches

This repository contains two important branches: `master` and `upstream-master`.

- `upstream-master` is a 1:1 copy of the `master` brach of the [upstream repository][upstream-repo]
- `master` follows branch `upstream-master` plus Giant Swarm specific changes

## Update from upstream

<details>

<summary>TL;DR - Give me just the commands</summary>

```
#!/bin/bash

set -e
set -x

git switch upstream-master
git pull https://github.com/cert-manager/cert-manager.git master --no-edit
git fetch https://github.com/cert-manager/cert-manager.git --tags
git switch master
git merge --no-edit upstream-master
vendir sync
git add vendir.lock.yml deploy/charts/cert-manager/templates/crds.yaml
git commit -m "Update CRDs from upstream"
git push origin master upstream-master
```

</details>

For procedure described below to work, you'll need to set up the upstream repository as remote `upstream`. You'll also need to have [vendir][vendir] installed.

Only once after cloning:

- `git remote add -f upstream https://github.com/cert-manager/cert-manager.git`
- Install [vendir][vendir]

Then every time you want to sync your `master` branch to upstream:

- Prepare your local `upstream-master`:
  - `git switch upstream-master`
  - `git pull upstream master --no-edit`
- Update your local `master`:
  - `git switch master`
  - `git merge --no-edit upstream-master`
- `git fetch upstream --tags` to update tags from upstream
- Run `vendir sync` to update CRDs
- Commit if changed `git add vendir.lock.yml deploy/charts/cert-manager/templates/crds.yaml && git commit -m "Update CRDs from upstream"`
- Push everything to the fork
  - `git push origin master upstream-master`
  - Push the lastest tag to our fork `git push origin vX.XX.X`

[upstream-repo]: https://github.com/cert-manager/cert-manager
[vendir]: https://github.com/carvel-dev/vendir

## Contributions

We want to contribute as much as possible to upstream.

### Which branch should I use as base?

For changes you don't want to contribute to upstream, start your work from `master`.

For changes you want to contribute to upstream, start your work from `upstream-master`.

### Which branch should I try to merge into?

Changes you don't want to contribute to upstream should merge into `master`.

Changes you want to contribute to upstream should be merged in upstream. Once merge in the upstream repository, update this repository as described in [Update from upstream](#update-from-upstream).

### My patch is urgent, what should I do?

In case your change is not useful to upstream, just create a branch from `master`, then after you are satisfied with your work, create a PR to merge into `master`.

In case you think the change is useful to upstream, create a branch from `upstream-master`. Create a first PR to merge into `master`. After the first PR is merged, create another PR to merge into upstream.
