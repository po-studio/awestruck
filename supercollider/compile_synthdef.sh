#!/bin/bash

INPUT_FILE=$1
OUTPUT_DIR=$2

if [ ! -f "$INPUT_FILE" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Usage: $0 <input_scd_file> <output_directory>"
    exit 1
fi

# Convert to absolute paths
INPUT_FILE=$(realpath "$INPUT_FILE")
OUTPUT_DIR=$(realpath "$OUTPUT_DIR")

# Create output directory with full permissions
mkdir -p "$OUTPUT_DIR" || true
chmod -R 777 "$OUTPUT_DIR" 2>/dev/null || true

# Run sclang in non-interactive mode to compile the synthdef
echo "Compiling synthdef..."

# Set environment variables to disable GUI and audio
export DISPLAY=""
export QT_QPA_PLATFORM="offscreen"
export SC_JACK_DEFAULT_OUTPUTS=""
export LANG=en_US.UTF-8
export NO_X11=1
export TERM=dumb

# Create a temporary file that sets up the environment
TMP_FILE=$(mktemp)
chmod 666 "$TMP_FILE" 2>/dev/null || true

cat > "$TMP_FILE" << EOF
(
// Basic server configuration
s = Server.local;
s.options.numOutputBusChannels = 2;
s.options.numInputBusChannels = 0;

// Load and interpret the file
{
    try {
        var file = File("$INPUT_FILE", "r");
        var code = file.readAllString;
        file.close;
        interpret(code);
    } { |error|
        error.errorString.postln;
        0.exit;
    };
}.value;

0.exit;
)
EOF

# Run sclang with the temporary file
cd "$OUTPUT_DIR" 2>/dev/null || true
sclang -r "$TMP_FILE"

# Clean up
rm "$TMP_FILE" 2>/dev/null || true

echo "Output directory contents:"
ls -la "$OUTPUT_DIR"

exit 0