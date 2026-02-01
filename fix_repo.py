#!/usr/bin/env python3
# Fix the repository.go file

with open('internal/repository/repository.go', 'rb') as f:
    content = f.read()

# Line 362 should have Count(®istered) but has Count(registered)
# where registered has \xc2\xae prefix
# Replace \xc2\xae with & to get Count(®istered)

# Find all Count calls and fix them
import re

# Convert to string for easier processing
text = content.decode('utf-8', errors='replace')

# Find the specific line
lines = text.split('\n')
for i, line in enumerate(lines):
    if 'Count(' in line and 'registered' in line and i > 350:
        print(f"Line {i+1}: {repr(line)}")
        # The line should be: Count(®istered)
        # But it might have the special character
        if '\u00ae' in line:  # ® character
            line = line.replace('\u00ae', '&')
            print(f"  Fixed to: {repr(line)}")
            lines[i] = line
        elif 'Count(&istered)' in line:
            line = line.replace('Count(&istered)', 'Count(®istered)')
            print(f"  Fixed to: {repr(line)}")
            lines[i] = line

# Write back
with open('internal/repository/repository.go', 'w', encoding='utf-8') as f:
    f.write('\n'.join(lines))

print("Done!")
