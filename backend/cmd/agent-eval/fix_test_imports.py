import os
import glob
import re

files = glob.glob('../../backend/cmd/agent-eval/*_test.go') + glob.glob('../../backend/internal/evals/*_test.go') + glob.glob('../../backend/internal/resumeeval/*_test.go')

for filepath in files:
    with open(filepath, 'r') as f:
        content = f.read()

    # Replace "github.com/allcallall/backend/internal/agent" with evals
    if '"github.com/allcallall/backend/internal/evals"' not in content:
        content = content.replace('"github.com/allcallall/backend/internal/agent"', '"github.com/allcallall/backend/internal/evals"')
        
    with open(filepath, 'w') as f:
        f.write(content)
