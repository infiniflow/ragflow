#!/bin/bash

# Change to the web3 directory
cd /Users/billzhang/Documents/GitHub/ragflow/web3

# Initialize the MySQL database
echo "Initializing MySQL database..."
docker exec -i web3-mysql mysql -uroot -ppassword logen_db < init.sql

# Check if initialization was successful
if [ $? -eq 0 ]; then
  echo "Database initialization completed successfully."
else
  echo "Database initialization failed. Exiting..."
  exit 1
fi

# Start the Node.js server
echo "Starting the Node.js server..."
node server/index.js 