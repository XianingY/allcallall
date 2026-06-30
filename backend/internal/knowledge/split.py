import re
import sys

def main():
    with open("service.go", "r") as f:
        content = f.read()

    # The file has imports and types at the top (lines 1 to ~120)
    # Then NewService, etc.
    # We will split it into multiple files, keeping `func (s *Service)` as the receiver to avoid complex dependency injection wiring.
    
    # We can identify functions by `^func `
    funcs = []
    
    # regex to find functions and their bodies
    # This is a bit tricky with nested braces, so let's do a simple brace counter.
    
    pos = 0
    header_end = content.find("func NewService")
    header = content[:header_end]
    
    # parse functions
    i = header_end
    while i < len(content):
        # find next func
        func_start = content.find("func ", i)
        if func_start == -1:
            break
        
        # find first {
        brace_start = content.find("{", func_start)
        if brace_start == -1:
            break
            
        # count braces
        braces = 1
        curr = brace_start + 1
        while curr < len(content) and braces > 0:
            if content[curr] == '{':
                braces += 1
            elif content[curr] == '}':
                braces -= 1
            curr += 1
            
        func_body = content[func_start:curr]
        funcs.append(func_body)
        i = curr

    def group_funcs(names):
        return [f for f in funcs if any(re.search(r'func \([^)]+\) ' + n + r'\(', f) or f.startswith(f"func {n}(") for n in names)]

    sources = group_funcs([
        "CreateSource", "UpdateSource", "ListSources", "ListSourceGroups", "GetSourceGroup",
        "CreateSourceGroup", "UpdateSourceGroup", "SetSourceGroupCanonical", "DeleteSource",
        "SyncActiveVersion", "GetSource", "ReingestSource", "loadSourcesByID", "loadVersionsByID",
        "markSourceFailed", "ensureSourceGroupTx", "nextVersionNumber", "prepareVersionForIngest", "nextVersionNumberTx", "createDuplicateCandidatesTx"
    ])
    
    chunks = group_funcs([
        "ListChunksBySource", "GetChunk", "DeleteChunksByVersion", "IndexChunk",
        "ProcessChunkIndex", "markChunkIndexFailed", "markChunkIndexSkipped", "loadActiveChunks"
    ])
    
    ingestion = group_funcs([
        "ProcessSourceIngest", "extractFileText", "ExtractFileText", "NormalizeText", "SimHashText", "validateFetchURL", "fetchURLText"
    ])
    
    duplicates = group_funcs([
        "ListDuplicateCandidates", "DecideDuplicateCandidate", "ProcessDuplicateDecision"
    ])
    
    search_bridge = group_funcs([
        "SearchRAG", "SearchRAGWithConversation", "Search", "searchResultsToOutput", "applyRerank"
    ])
    
    # core is everything else
    assigned = set(sources + chunks + ingestion + duplicates + search_bridge)
    core = [f for f in funcs if f not in assigned]
    
    def write_file(name, funcs_to_write, extra_imports=""):
        with open(name, "w") as f:
            f.write("package knowledge\n\n")
            f.write("import (\n")
            f.write('\t"context"\n')
            f.write('\t"time"\n')
            f.write('\t"net/http"\n')
            f.write('\t"strings"\n')
            f.write('\t"github.com/allcallall/backend/internal/models"\n')
            f.write('\t"gorm.io/gorm"\n')
            f.write(extra_imports)
            f.write(")\n\n")
            f.write("\n\n".join(funcs_to_write))
            f.write("\n")
            
    write_file("source_service.go", sources)
    write_file("chunk_service.go", chunks)
    write_file("ingestion_pipeline.go", ingestion, '\t"io"\n\t"html"\n\t"mime"\n\tpdf "github.com/ledongthuc/pdf"\n\t"bytes"\n\t"crypto/sha256"\n\t"encoding/hex"\n\t"hash/fnv"\n\t"math/bits"\n\t"regexp"\n\t"unicode"\n\t"net/url"\n')
    write_file("duplicate_detector.go", duplicates)
    write_file("search_bridge.go", search_bridge, '\t"github.com/allcallall/backend/internal/search"\n\t"sort"\n')
    
    with open("service.go", "w") as f:
        f.write(header)
        f.write("\n\n".join(core))
        f.write("\n")

if __name__ == "__main__":
    main()
