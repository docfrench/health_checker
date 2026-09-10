#!/bin/bash
docker inspect --format='{{.State.Running}}' NginxProxyManager
