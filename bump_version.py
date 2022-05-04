import sys
import os
from yaml import full_load, dump

manifest_path = 'manifest.yml'
stream = open(file=manifest_path, mode='r')
manifest = full_load(stream)

# print current version and exit without bumping
if len(sys.argv) >= 2 and sys.argv[1] == "read_only":
    print(manifest['docker']['external_version'])
    sys.exit()

(major, minor) = manifest['docker']['external_version'].split(".")

minor = str(int(minor) + 1)

bumped_version = f'{major}.{minor}'

manual_version = os.getenv('VERSION_TO_PUBLISH', '')

if manual_version != '':
    bumped_version = manual_version

print(bumped_version)

manifest['docker']['external_version'] = bumped_version
stream = open(file=manifest_path, mode='w')
dump(data=manifest, stream=stream)

