#!/usr/bin/python3
#
# PTP's Archiver Client Utility
#
# This utility allows you to allocate "containers" that PTP will provide neglected torrents to
# archive in. You cannot control what content is put in those containers.
# Using this script is done with subcommands and flags.
# e.g. python archiver.py subcommand arg0 arg1 ... --flag0 --flag1
#
# Subcommands:
#   setup     create a new configuration file (required)
#   list      list the containers currently configured
#   create    create a new container
#               arg0: the name to give this container (required)
#   fetch     fetch a new torrent for a container
#               arg0: the name of the container to fetch for or all for all containers (required)
#   update    attempt to update this script if necessary
#
# Flags:
#   --config  the path to a configuration file (if not provided, ./config.ptp is used)
#             if this flag is provided during setup, that is where the config will be saved to
#
# Typical usage:
#   python archiver.py setup
#     This should be done once, after you've downloaded this script
#   python archiver.py fetch MyContainer
#     This downloads a new .torrent and places it in the WatchDirectory found in your config
#
# Config:
#   The config file has settings that you may want to change, including:
#     Default         these are the default settings for new containers
#     WatchDirectory  this is the directory where new .torrents for this container will go
#     AfterFetchExec  this is a shell command that you can set to trigger after a new download
#                     see the default_config below for comments on variables
#   Those settings are acceptable to change in the config file directly, but most other settings
#   should *only* be changed through this script because they necessitate server interaction.
#
# Third-party dependencies: requests
# Note: Python3 is *highly* recommended but python2 is mostly supported.
#

import os
import re
import cgi
import sys
import copy
import json
import time
import shutil
import argparse
import requests
import subprocess

# Needed for python2 backwards compatibility.
if sys.version_info[0] < 3: input = raw_input  # pylint: disable undefined-variable


__version__ = '0.10'

global_flags = {
  '--config': {'default': 'config.ptp', 'help': 'path to a configuration file to read/write'},
}

actions = {
  'create': {
    'help': 'create a new container using a supplied name',
    'flags': {'name': {'help': 'the name for the new container'}}
  },
  'fetch': {
    'help': 'fetch more torrents for a given container',
    'flags': {'name': {'help': 'the name of the container to add to (or all)'}}
  },
  'list': {
    'help': 'output the list of configured containers',
    'flags': {},
  },
  # TODO
  # 'remove': {
  #   'help': 'remove a given container by name',
  #   'flags': {'name': {'help': 'the name of the container to remove'}}
  # },
  # TODO
  # 'rename': {
  #   'help': 'rename a given container by name',
  #   'flags': {
  #     'name': {'help': 'the old name of the container'},
  #     'new_name': {'help': 'the new name for the container'},
  #   }
  # },
  # TODO
  # 'resize': {
  #   'help': 'resize a given container by name',
  #   'flags': {
  #     'name': {'help': 'the name of the container to remove'},
  #     'size': {'help': 'the size (in G, T, etc.) of the container'},
  #   },
  # },
  'setup': {
    'help': 'setup a new archiver environment',
    'flags': {}
  },
  'update': {
    'help': 'update this script if necessary',
    'flags': {},
  },
}

intro = (
  'Welcome to PTP\'s Archiver Script\n\n'
  'Basic usage: python archiver.py <option>\n\n'
  'Options:\n'
  '%s\n\n' % '\n'.join('\t%s\t%s' % (a, f['help']) for a, f in actions.items()) +
  'Flags (--flag):\n'
  '\thelp\tget help with this script or with any option (e.g. update --help)\n'
  '%s\n\n' % '\n'.join('\t%s\t%s' % (a.replace('--', ''), f['help']) for a, f in global_flags.items()) +
  'If this is your first time using this script, try "python archiver.py setup"'
)

default_config = {
  'ApiKey': None,
  'ApiUser': None,
  'Containers': {},
  'BaseURL': 'https://passthepopcorn.me',
  'FetchURL': 'archive.php?action=fetch',
  'UpdateURL': 'archive.php?action=script',
  'VersionURL': 'archive.php?action=scriptver',
  'DownloadURL': 'torrents.php?action=download',

  # Time to wait when fetching for multiple containers at once (strongly recommend >3 seconds).
  'FetchSleep': 5,

  'Default': {
    'Size': '500G',
    'MaxStalled': 0,
    # Variables for an AfterFetchExec:
    #   $path: replaced with the path to the newly fetched
    #   $name: replaced with the base name of the newly fetched
    'AfterFetchExec': None,
    # Variables for a WatchDirectory:
    #   $name: replaced with the name of the container
    'WatchDirectory': '/tmp/$name',
  },
}


def save_config(path, config):
  """Save the config dictionary to a given path."""
  with open(path, 'w') as config_file:
    json.dump(config, config_file, indent=2, sort_keys=True)

def load_config(path):
  """Load the config dictionary from a given path.
  This also merges new settings (from script changes) with potentially older config files.
  """
  if not os.path.exists(path):
    raise ValueError('Cannot locate configuration file (%s)' % path)

  config = json.load(open(path, 'r'))
  for key in default_config:
    if key not in config:
      config[key] = default_config[key]

  for name, container in config['Containers'].items():
    for key in default_config['Default']:
      if key not in container:
        container[key] = default_config['Default'][key]
    container['WatchDirectory'] = container['WatchDirectory'].replace('$name', name)

  return config

def accept_or_die(question):
  """Forces the user to accept a condition or the program will terminate."""
  if input('%s (y/N)? ' % question).lower() not in ('y', 'yes'):
    sys.exit()

def request(config, relurl, params, **kwargs):
  """Makes a request to BaseURL+RelativeURL using the user's Api credentials."""
  headers = {'ApiUser': config['ApiUser'], 'ApiKey': config['ApiKey']}
  url = requests.compat.urljoin(config['BaseURL'], relurl)

  req = requests.get(url, headers=headers, params=params, **kwargs)
  req.raise_for_status()
  return req

def api_request(config, relurl, params, *keys):
  """Makes a standard server request with error checking."""
  reply = request(config, relurl, params).json()

  if 'Status' not in reply or reply['Status'] not in ('Ok', 'Error') or [k for k in keys if k not in reply]:
    raise RuntimeError('Error: Unexpected server response\n\n%s' % reply)
  return reply

def fetch_request(config, name):
  """Fetch a new torrent for a given container, selected by PTP."""
  if name not in config['Containers']:
    raise ValueError('No container with the name "%s"' % name)
  container = config['Containers'][name]
  params = {'ContainerName': name, 'ContainerSize': container['Size'], 'MaxStalled': container['MaxStalled']}

  reply = api_request(config, config['FetchURL'], params, 'ContainerID', 'ScriptVersion')

  # The ContainerID may be useful later (e.g. for renaming) so store it if possible
  container['ContainerID'] = int(reply['ContainerID'])
  if float(reply['ScriptVersion']) > float(__version__):
    print('This script is currently out-of-date, use "python archiver.py update" to update')
    print('Current version is %s while newest version is %s' % (__version__, reply['ScriptVersion']))

  return reply

def download_request(config, name, fetch):
  """Using fetch results, download a new torrent to a given container."""
  if name not in config['Containers']:
    raise ValueError('No container with the name "%s"' % name)
  container = config['Containers'][name]
  params = {'id': fetch['TorrentID'], 'ArchiveID': fetch['ArchiveID']}

  reply = request(config, config['DownloadURL'], params, stream=True)

  content = cgi.parse_header(reply.headers.get('Content-Disposition', ''))[-1]
  if 'filename' not in content:
    raise RuntimeError('Error: Unexpected server response\n\n%s' % reply.headers)
  filename = os.path.basename(content['filename'])

  abs_path = os.path.abspath(container['WatchDirectory'])
  if not os.path.isdir(abs_path):
    accept_or_die('WatchDirectory "%s" does not exist, create' % abs_path)
    os.makedirs(abs_path)
  abs_path = os.path.join(abs_path, filename)

  with open(abs_path, 'wb') as target:
    reply.raw.decode_content = True
    shutil.copyfileobj(reply.raw, target)

  return abs_path

def update_request(config):
  """Checks if there's a new version of this script and attempts to update if possible.
  This version will be backed up as scriptname-version.bak."""
  reply = api_request(config, config['VersionURL'], (), 'ScriptVersion')

  if float(reply['ScriptVersion']) <= float(__version__):
    return

  accept_or_die('Update from %s to %s' % (__version__, reply['ScriptVersion']))
  reply = request(config, config['UpdateURL'], (), stream=True)

  abs_path = os.path.abspath(sys.argv[0])
  os.rename(abs_path, '%s-%s.bak' % (abs_path, __version__))

  with open(abs_path, 'wb') as target:
    reply.raw.decode_content = True
    shutil.copyfileobj(reply.raw, target)

  return abs_path


if __name__ == '__main__':
  # Handle command line argument parsing.
  parser = argparse.ArgumentParser()
  subparsers = parser.add_subparsers(dest='action')
  for action in actions:
    subparser = subparsers.add_parser(action, help=actions[action]['help'])
    flags = actions[action]['flags']
    flags.update(global_flags)
    for flag in flags:
      subparser.add_argument(flag, **flags[flag])
  args = parser.parse_args()

  if args.action is None:
    print(intro)
    sys.exit()

  # Configure a new environment if requested.
  if args.action == 'setup':
    if os.path.exists(args.config):
      accept_or_die('Overwrite %s' % os.path.abspath(args.config))

    config = copy.deepcopy(default_config)

    user = input('ApiUser: ')
    if not re.match(r'^[\w\d]{16}$', user):
      print('Invalid user, recheck your account settings')
      sys.exit()

    key = input('ApiKey:  ')
    if not re.match(r'^[\w\d]{32}$', key):
      print('Invalid key, recheck your account settings')
      sys.exit()

    config['ApiUser'], config['ApiKey'] = user, key
    save_config(args.config, config)
    print('New config written to %s' % os.path.abspath(args.config))

    accept_or_die('Create a new container now')
    args.name = input('Name: ')
    args.action = 'create'


  # Load the configuration.
  config = load_config(args.config)

  # List the containers if requested.
  if args.action == 'list':
    print("Containers:")
    for n, c in config['Containers'].items():
      print('\t%s (%s): %s' % (n, c['Size'], c['WatchDirectory']))

  # Remove a container if requested.
  if args.action == 'remove':
    if args.name not in config['Containers']:
      print('No container with the name "%s"' % args.name)
      sys.exit()
    del config['Containers'][args.name]

  # Resize a container if requested.
  if args.action == 'resize':
    size = args.size.upper()
    if not re.match(r'^\d+[BKMGTP]$', size):
      print('Requested size is invalid (should be #U where U is G, T, etc.)')
      sys.exit()
    if args.name not in config['Containers']:
      print('No container with the name "%s"' % args.name)
      sys.exit()
    config['Containers'][args.name]["Size"] = size

  # Create a container if requested.
  if args.action == 'create':
    if args.name in config['Containers']:
      print('Container with the name "%s" already exists' % args.name)
      sys.exit()
    container = copy.deepcopy(config['Default'])
    config['Containers'][args.name] = container

  # Fetch items for the container if requested.
  if args.action == 'fetch':
    if args.name.lower() == 'all':
      containers = sorted(config['Containers'].keys())
    else:
      containers = [args.name]
    for container in containers:
      fetch = fetch_request(config, container)
      filepath = download_request(config, container, fetch)
      print('New download written as "%s"' % filepath)

      after = config['Containers'][container]['AfterFetchExec']
      if after:
        after = after.replace('$path', filepath)
        after = after.replace('$name', os.path.basename(filepath))
        print('Running post-fetch exec: "%s"\n' % after)

        # Subprocess does not/should not use the shell so treat '&&' as separate commands if
        # it is being used.
        for cmd in after.split('&&'):
          subprocess.Popen(cmd.split()).communicate()

      if containers[-1] != container:
        time.sleep(config['FetchSleep'])

  # Try to update this script if requested.
  if args.action == 'update':
    filepath = update_request(config)
    if filepath:
      print('Updated (%s)' % filepath)
    else:
      print('Script is currently up-to-date')

  save_config(args.config, config)