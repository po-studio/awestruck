(
s.waitForBoot({
    Ndef('x', {
        var noise = LFNoise1.ar(0.1, 3); // Low frequency noise modulation
        var osc = SinOsc.ar(65, Ndef('x').ar * noise, LFNoise1.ar(3, 6)); // Sinusoidal oscillator with frequency modulation
        var processed = osc.tanh; // Apply hyperbolic tangent for distortion
        9.do {
            processed = AllpassL.ar(processed, 0.3, {0.2.rand + 0.1}!2, 5); // Apply all-pass filter in a loop for reverb effect
        };
        processed.tanh; // Final distortion
    }).play;
});
)
