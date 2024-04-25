(
s.waitForBoot({
    play {
        var gain = 0.25;  // Master output gain
        var d = Duty;  // Duty cycle generator
        
        // Create a control signal from a sequence of MIDI notes incremented by 36 semitones
        var f = d.kr(
            1 / [1, 2, 3],  // Periods for the duty cycle at control rate
            0,  // Phase of the duty cycle
            Dseq([0, 3, 7, 12, 8] + 36, inf)  // Sequence of MIDI notes, repeated indefinitely
        );

        // Reverb applied to a sum of frequency-modulated Blip oscillators
        GVerb.ar(
            Blip.ar(
                f.midicps * [1, 2, 3],  // Convert MIDI sequence to cps and create harmonics
                LFNoise1.kr(1, 8, 8)  // Modulate the number of harmonics with noise
            ).sum,  // Sum the outputs of the Blip oscillators
            99,  // Room size for the reverb
            4   // Damping factor in the reverb
        ) * gain;  // Scale the output by the gain factor
    }
});
)
