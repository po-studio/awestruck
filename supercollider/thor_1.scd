(
s.waitForBoot({
    play {
        var x = SinOsc;  // Sine wave oscillator
        var y = LFNoise0;  // Low-frequency noise generator
        
        var a = y.ar(8);  // Audio rate noise modulation
        
        // Combine multiple oscillators and process through a MoogFF filter
        var output = (
            x.ar(Pulse.ar(1) * 24) +  // Sine wave modulated by a pulse wave
            x.ar(90 + (a * 90)) +  // Frequency-modulated sine wave
            MoogFF.ar(
                Saw.ar(y.ar(4, 333, 666)),  // Saw wave modulated by another LFNoise
                a * XLine.ar(1, 39, 99, 99, 0, 2)  // Filter cutoff modulated by a scaled exponential line
            )
        ) ! 2 / 3;  // Duplicate for stereo output and normalize volume
        
        output;  // Play the combined output
    };
});
);