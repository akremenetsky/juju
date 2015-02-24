#!/bin/bash
# This script presumes ~/ and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))
JUJU_HOME=${JUJU_HOME:-$(dirname $SCRIPTS)/cloud-city}

SSH_OPTIONS="-i $JUJU_HOME/staging-juju-rsa \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"


usage() {
    echo "usage: $0 user@host"
    echo "  user@host: The user and host to ssh to."
    exit 1
}


test $# -eq 1 || usage
USER_AT_HOST="$1"

set -x
ssh $SSH_OPTIONS $USER_AT_HOST "revision_build=$revision_build bash" <<"EOT"
#!/bin/bash
set -eu
RELEASE_SCRIPTS=$HOME/juju-release-tools
SCRIPTS=$HOME/juju-ci-tools
GOBASE=$HOME/crossbuild
WORKSPACE=$HOME/workspace
JUJU_HOME=$HOME/.juju
source $HOME/.bashrc
source $HOME/cloud-city/juju-qa.jujuci
set -x

cd $WORKSPACE
$SCRIPTS/jujuci.py -v setup-workspace $WORKSPACE
TARFILE=$($SCRIPTS/jujuci.py get build-revision 'juju-core_*.tar.gz' ./)
echo "Downloaded $TARFILE"
$RELEASE_SCRIPTS/crossbuild.py -v osx-client -b $GOBASE ./$TARFILE
EOT
EXIT_STATUS=$?

if [ $EXIT_STATUS -eq 0 ]; then
    scp $SSH_OPTIONS \
        $USER_AT_HOST:~/workspace/juju-*-osx.tar.gz $WORKSPACE/artifacts/
fi

exit $EXIT_STATUS
