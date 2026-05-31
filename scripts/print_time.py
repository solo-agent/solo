#!/usr/bin/env python3
"""Print the current time."""

from datetime import datetime

now = datetime.now()
print(now.strftime('%Y-%m-%d %H:%M:%S'))
