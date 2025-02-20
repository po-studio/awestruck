#!/bin/bash

INPUT_FILE=$1
OUTPUT_DIR=$2

if [ ! -f "$INPUT_FILE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 <input_scd_file> <output_directory>"
    exit 1
fi

echo "=== SuperCollider SynthDef Compilation ==="
echo "Input file: $INPUT_FILE"
echo "Output directory: $OUTPUT_DIR"

# Convert to absolute paths
INPUT_FILE=$(realpath "$INPUT_FILE")
OUTPUT_DIR=$(realpath "$OUTPUT_DIR")
echo "Absolute paths:"
echo "- Input: $INPUT_FILE"
echo "- Output: $OUTPUT_DIR"

# Create output directory with full permissions
echo "Creating output directory..."
mkdir -p "$OUTPUT_DIR" || { echo "Failed to create output directory"; exit 1; }
chmod -R 777 "$OUTPUT_DIR" 2>/dev/null || echo "Warning: Could not set directory permissions"

# Create a temporary file for the compilation
TMP_FILE=$(mktemp)
echo "Created temporary script at: $TMP_FILE"
chmod 666 "$TMP_FILE" 2>/dev/null || echo "Warning: Could not set temp file permissions"

cat > "$TMP_FILE" << EOF
(
"Starting SynthDef compilation...".postln;
try {
    var file = File("$INPUT_FILE", "r");
    var code = file.readAllString;
    file.close;
    
    "Loaded source file successfully".postln;
    
    "Evaluating SynthDef code...".postln;
    
    // Replace .add with .writeDefFile
    code = code.replace("}).add;", "}).writeDefFile(\"$OUTPUT_DIR\");");
    
    // Evaluate the modified code
    thisProcess.interpreter.interpret(code);
    
    "SynthDef compilation completed".postln;
    
} { |error|
    "ERROR during compilation:".postln;
    error.errorString.postln;
    1.exit;  // Exit with error code 1
};

0.exit;
)
EOF

# Run sclang with the temporary file
echo "Running SuperCollider compiler..."
cd "$OUTPUT_DIR" 2>/dev/null || { echo "Failed to change to output directory"; exit 1; }
sclang -r "$TMP_FILE"
SCLANG_EXIT=$?

# Clean up
rm "$TMP_FILE" 2>/dev/null || echo "Warning: Could not remove temporary file"

echo "SuperCollider compiler exit code: $SCLANG_EXIT"
echo "Output directory contents:"
ls -la "$OUTPUT_DIR"

if [ $SCLANG_EXIT -ne 0 ]; then
    echo "ERROR: SuperCollider compilation failed"
    exit 1
fi

# Verify that at least one .scsyndef file was created
if ! ls "$OUTPUT_DIR"/*.scsyndef >/dev/null 2>&1; then
    echo "ERROR: No .scsyndef files were created"
    exit 1
fi

echo "=== Compilation completed ==="
exit 0