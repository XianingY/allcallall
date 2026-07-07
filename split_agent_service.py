import re

with open('backend/internal/agent/service.go', 'r') as f:
    lines = f.readlines()

functions_to_extract_lifecycle = [
    "RunConversationAssistant",
    "agentRunSource",
    "findRunByIdempotencyKey",
    "GetRun",
    "GetRunEvents",
    "ExecuteRun",
    "executeLegacyAgentRun"
]

functions_to_extract_persistence = [
    "buildRunResult",
    "createStep",
    "recordContextToolCalls",
    "recordAgentToolCalls",
    "executeSideEffectTools",
    "appendToolCall",
    "recordToolCallError",
    "loadConversationContext"
]

def extract_funcs(lines, func_names):
    extracted = []
    remaining = []
    in_func = False
    current_func = []
    brace_count = 0
    
    for line in lines:
        if not in_func:
            match = re.match(r'^func (?:(?:[a-zA-Z0-9_\*\s\(\)]+) )?([a-zA-Z0-9_]+)\(', line)
            if match and match.group(1) in func_names:
                in_func = True
                current_func = [line]
                brace_count = line.count('{') - line.count('}')
            else:
                remaining.append(line)
        else:
            current_func.append(line)
            brace_count += line.count('{') - line.count('}')
            if brace_count == 0:
                in_func = False
                extracted.extend(current_func)
                extracted.append("\n")
                
    return extracted, remaining

extracted_lifecycle, remaining = extract_funcs(lines, functions_to_extract_lifecycle)
extracted_persistence, remaining = extract_funcs(remaining, functions_to_extract_persistence)

with open('backend/internal/agent/service.go', 'w') as f:
    f.writelines(remaining)

imports = """package agent

import (
\t"context"
\t"encoding/json"
\t"errors"
\t"fmt"
\t"strings"
\t"time"

\t"gorm.io/gorm"

\t"github.com/allcallall/backend/internal/events"
\t"github.com/allcallall/backend/internal/knowledge"
\t"github.com/allcallall/backend/internal/models"
\t"github.com/allcallall/backend/internal/search"
\t"github.com/allcallall/backend/internal/trace"
)

"""

with open('backend/internal/agent/lifecycle.go', 'w') as f:
    f.write(imports)
    f.writelines(extracted_lifecycle)

with open('backend/internal/agent/persistence.go', 'w') as f:
    f.write(imports)
    f.writelines(extracted_persistence)

print("Split completed.")
