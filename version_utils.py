import sys
import os
from yaml import full_load, dump

CMD_READ = 'read'
CMD_BUMP = 'bump'
VERSION_ENV_VAR = 'VERSION_TO_PUBLISH'

if len(sys.argv) < 2:
    print("missing command")
    sys.exit(-1)

command = sys.argv[1]
available_commands = [CMD_BUMP, CMD_READ]

if command not in available_commands:
    print(f"Unsupported command - {command} . available commands: {available_commands}")
    sys.exit(-1)

manifest_path = 'manifest.yml'
stream = open(file=manifest_path, mode='r')
manifest = full_load(stream)


if command == CMD_READ:
    print(manifest['docker']['external_version'])
    sys.exit()

if command == CMD_BUMP:
    (major, minor) = manifest['docker']['external_version'].split(".")

    minor = str(int(minor) + 1)

    bumped_version = f'{major}.{minor}'

    manual_version = os.getenv(VERSION_ENV_VAR, '')

    if manual_version != '':
        bumped_version = manual_version
        print(f'taking version from {VERSION_ENV_VAR}')

    print(bumped_version)

    manifest['docker']['external_version'] = bumped_version
    stream = open(file=manifest_path, mode='w')
    dump(data=manifest, stream=stream)

