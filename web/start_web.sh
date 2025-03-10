#!/bin/bash
pm2 start npm --name "ragflow-web" -- run dev
pm2 status ragflow-web