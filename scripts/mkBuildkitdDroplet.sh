#!/bin/bash
set -e

# TODO add flags for --sshkey, --size, --region, --name, --image
# TODO check for curl and jq binaries

REPO="$1"
SSH="$2"
if [[ -z $REPO || -z $SSH ]]; then
    echo "Usage: $0 <repo url> <ssh key fingerprint>" >&2
    echo >&2
    echo "For example to add buildkitd actions runner to openllb/hlb project you would run:" >&2
    echo >&2
    echo "	$0 https://github.com/openllb/hlb 3f:2a:4b:7d:d9:46:9f:43:99:3d:c3:48:bb:62:f3:a4" >&2
    echo >&2
    exit 1
fi

DIGITAL_OCEAN_TOKEN=$(cat ~/.digitalocean-token)

if [[ -z $DIGITAL_OCEAN_TOKEN ]]; then
    echo "Missing ~/.digitalocean-token file which must contain an API token for DigitalOcean" >&2
    echo "To create a token go here: https://cloud.digitalocean.com/account/api/tokens and select" >&2
    echo "\"Generate New Token\", then run:" >&2
    echo >&2
    echo "	echo \"put-token-here\" > $HOME/.digitalocean-token" >&2
    echo >&2
    exit 1
fi

GITHUB_TOKEN=$(cat ~/.github-repo-token)

if [[ -z $GITHUB_TOKEN ]]; then
    echo "Missing ~/.github-repo-token file which must contain an API token for Github" >&2
    echo "To create a token go here: https://github.com/settings/tokens and select" >&2
    echo "\"Generate new token\", then under \"Selected scopes\" check the \"repo\" box."  >&2
    echo >&2
    echo "	echo \"put-token-here\" > $HOME/.github-repo-token" >&2
    echo >&2
    exit 1
fi

# get short-lived token to allow actions runner to register with Github
REGISTER_TOKEN=$(curl -qsfL -X POST -H 'Authorization: token '$GITHUB_TOKEN'' https://api.github.com/repos/openllb/hlb/actions/runners/registration-token | jq -r .token)

# Use github api to find latest release tarball location
ACTIONS_RUNNER_TGZ=$(curl -qsLf https://api.github.com/repos/actions/runner/releases/latest | jq -r '.assets[] | select(.name | contains("linux")) | select(.name | contains("x64")) | .browser_download_url')

USERDATA=$(cat <<EOM
#cloud-config
runcmd:
  - |
    # setup firewall to only allow in ssh/22
    ufw default deny incoming
    ufw default allow outgoing
    ufw allow ssh
    ufw enable
  - |
    # install docker & latest git
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
    add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu \$(lsb_release -cs) stable"
    add-apt-repository ppa:git-core/ppa
    apt-get update
    apt-get install -y docker-ce docker-ce-cli containerd.io build-essential git
  - |
    # install actions-runner
    useradd -g docker --create-home --shell /usr/sbin/nologin github
    mkdir -p /github
    chown github /github
    sudo -u github mkdir -p /github/actions-runner /github/work
    cd /github/actions-runner
    curl -qsfL $ACTIONS_RUNNER_TGZ | sudo -u github tar xzf -
    sudo -u github ./config.sh \
        --unattended \
        --replace \
        --name buildkitd \
        --labels buildkitd  \
        --work ../work \
        --url $REPO \
        --token $REGISTER_TOKEN
    sudo -H -u github nohup ./run.sh &
  - docker run -d --name buildkitd --privileged moby/buildkit:latest
EOM
)

# some valid sizes we might want to try:
# "s-1vcpu-1gb" => $5/mo
# "s-1vcpu-2gb" => $10/mo
# "s-3vcpu-1gb" => $15/mo
# "s-2vcpu-2gb" => $15/mo
# "s-1vcpu-3gb" => $15/mo
# "s-2vcpu-4gb" => $20/mo

PAYLOAD=$(cat <<'EOM'
{
    name: "buildkitd",
    region: "nyc3",
    size: "s-2vcpu-2gb",
    image: "ubuntu-18-04-x64",
    ssh_keys: [$sshkey],
    user_data: $userdata
}
EOM
)

jq -n --arg userdata "$USERDATA" --arg sshkey "$SSH" "$PAYLOAD" | \
    curl -qsfL -X POST \
        -H 'Content-Type: application/json' \
        -H 'Authorization: Bearer '$DIGITAL_OCEAN_TOKEN'' \
        "https://api.digitalocean.com/v2/droplets" \
        -d@- | jq .
