#!/bin/bash

# docker buildx build --platform linux/amd64 -t ghcr.io/bareckidarek/deep_in_memory_playground:$TAG --push .
#labctl playground update deep-in-memory-playground-29f48c57 &
#pid1=$!

labctl content push -fw tutorial deep-in-memory-tutorial-ac080898 &
pid2=$!

cleanup() {
  echo "Cleaning up..."
  #kill "$pid1" "$pid2" 2>/dev/null
  #wait "$pid1" "$pid2" 2>/dev/null

  kill "$pid2" 2>/dev/null
  wait "$pid2" 2>/dev/null
}

trap cleanup EXIT INT TERM  

#wait $pid1 $pid2
wait $pid2