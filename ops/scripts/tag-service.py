#!/usr/bin/env python3
import logging.config
import os
import re
import subprocess
import sys

from packaging.version import Version

# Minimum version numbers for packages migrating from legacy versioning.
MIN_VERSIONS = {
    'op-node': '0.10.14',
    'op-batcher': '0.10.14',
    'op-proposer': '0.10.14',
    'proxyd': '3.16.0',
    'indexer': '0.5.0'
}

VALID_BUMPS = ('major', 'minor', 'patch')

MESSAGE_TEMPLATE = '[tag-service-release] Tag {service} at {version}'

LOGGING_CONFIG = {
    'version': 1,
    'disable_existing_loggers': True,
    'formatters': {
        'standard': {
            'format': '%(asctime)s [%(levelname)s]: %(message)s'
        },
    },
    'handlers': {
        'default': {
            'level': 'INFO',
            'formatter': 'standard',
            'class': 'logging.StreamHandler',
            'stream': 'ext://sys.stderr'
        },
    },
    'loggers': {
        '': {
            'handlers': ['default'],
            'level': 'INFO',
            'propagate': False
        },
    }
}

logging.config.dictConfig(LOGGING_CONFIG)
log = logging.getLogger(__name__)


def main():
    bump = sys.argv[1]
    service = sys.argv[2]
    if bump not in VALID_BUMPS:
        die('bump must be "major" "minor" or "patch"')
    if service not in MIN_VERSIONS:
        die('service must be one of: {}'.format(', '.join(MIN_VERSIONS.keys())))

    tags = subprocess.run(['git', 'tag', '--list'], capture_output=True, check=True) \
        .stdout.decode('utf-8').splitlines()

    version_pattern = f'^{service}/v\\d+\\.\\d+\\.\\d+$'
    svc_versions = [t.replace(f'{service}/v', '') for t in tags if re.match(version_pattern, t)]
    svc_versions.sort(key=Version)
    svc_versions.reverse()

    if len(svc_versions) == 0:
        latest_version = MIN_VERSIONS[service]
    else:
        latest_version = svc_versions[0]

    log.info(f'Latest version: v{latest_version}')

    components = latest_version.split('.')
    if bump == 'major':
        components[0] = str(int(components[0]) + 1)
        components[1] = '0'
        components[2] = '0'
    if bump == 'minor':
        components[1] = str(int(components[1]) + 1)
        components[2] = '0'
    elif bump == 'patch':
        components[2] = str(int(components[2]) + 1)
    else:
        raise Exception('Invalid bump type: {}'.format(bump))

    new_version = 'v' + '.'.join(components)
    new_tag = f'{service}/{new_version}'

    log.info(f'Bumped version: {new_version}')

    log.info('Configuring git')
    # The below env vars are set by GHA.
    gh_actor = os.environ['GITHUB_ACTOR']
    gh_token = os.environ['INPUT_GITHUB_TOKEN']
    gh_repo = os.environ['GITHUB_REPOSITORY']
    origin_url = f'https://{gh_actor}:${gh_token}@github.com/{gh_repo}.git'
    subprocess.run(['git', 'config', 'user.name', gh_actor], check=True)
    subprocess.run(['git', 'config', 'user.email', f'{gh_actor}@users.noreply.github.com'], check=True)
    subprocess.run(['git', 'remote', 'set-url', 'origin', origin_url], check=True)

    log.info(f'Creating tag: {new_tag}')
    subprocess.run([
        'git',
        'tag',
        '-a',
        new_tag,
        '-m',
        MESSAGE_TEMPLATE.format(service=service, version=new_version)
    ], check=True)

    log.info('Pushing tag to origin')
    subprocess.run(['git', 'push', 'origin', new_tag], check=True)


def cmp_version(a, b):
    if Version(a) < Version(b):
        return -1
    elif Version(a) > Version(b):
        return 1
    else:
        return 0


def die(msg):
    print(msg)
    sys.exit(1)


if __name__ == '__main__':
    main()
