
SynthDef.new("openai-o1-preview-2024_12_19_03_43_09", { |out=0, amp=0.5|
    var sound;
    var sound, pad, padFreqs, padOscs, scale, melody, melodyTrig, melodyNote, melodyFreq, noiseTexture, rhythmicPulse, pulseTrig, pulseFreq, bell, bellTrig, bellFreq;

scale = [57, 59, 60, 62, 64, 65, 67]; // A minor scale MIDI numbers

// Pad frequencies
padFreqs = Array.fill(4, {
    var trig = Impulse.kr(1 / (20 + 20.rand)); // change every 20 to 40 seconds
    var note = Demand.kr(trig, 0, Dseq(scale, inf));
    Lag.kr(note.midicps, 10); // glide over 10 seconds
});

padOscs = padFreqs.collect { |freq, i|
    var osc = Select.kr(i % 2, [SinOsc.ar(freq), Saw.ar(freq)]);
    var amp = 0.1 + LFNoise1.kr(0.03).range(0, 0.05);
    osc * amp;
};

pad = Mix.ar(padOscs);

// Melody
melodyTrig = Impulse.kr(1 / (15 + 10.rand)); // every 15 to 25 seconds
melodyNote = Demand.kr(melodyTrig, 0, Dshuf(scale, inf));
melodyFreq = Lag.kr(melodyNote.midicps, 5);

melody = SinOsc.ar(melodyFreq, 0, 0.08) * EnvGen.kr(Env.perc(1, 4), melodyTrig);

// Noise texture
noiseTexture = PinkNoise.ar(0.02);
noiseTexture = RLPF.ar(noiseTexture, 500 + LFNoise1.kr(0.05).range(-300, 300), 0.3);

// Rhythmic pulse
pulseTrig = Dust.kr(0.03);
pulseFreq = Demand.kr(pulseTrig, 0, Dshuf(scale + Dseq([0, 12], inf), inf));
rhythmicPulse = SinOsc.ar(pulseFreq.midicps, 0, 0.05) * EnvGen.kr(Env.perc(0.01, 1), pulseTrig);

// Bell-like tones
bellTrig = Dust.kr(0.05);
bellFreq = Demand.kr(bellTrig, 0, Dshuf(scale + 24, inf));
bell = SinOsc.ar(bellFreq.midicps, 0, 0.1) * EnvGen.kr(Env.perc(0.01, 2), bellTrig);

// Sum all components
sound = pad + melody + noiseTexture + rhythmicPulse + bell;

// Add subtle modulation
sound = sound * (0.5 + LFNoise1.kr(0.02).range(-0.1, 0.1));

// Apply reverb and delay
sound = FreeVerb.ar(sound, 0.5, 0.8);
sound = DelayN.ar(sound, 0.5, 0.3 + LFNoise1.kr(0.1).range(-0.1, 0.1));

// Limit amplitude
sound = Limiter.ar(sound, 0.5);
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
