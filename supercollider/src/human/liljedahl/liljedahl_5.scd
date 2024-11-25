(
s.waitForBoot({
  play {
    var gain = 0.25;  
    
    GVerb.ar(
      GVerb.ar(
        Impulse.ar([8, 6, 4] / 100, [0, 0.2, 0.6]) * 8,
        [2, 3, 1.5],  
        90,  
        dryLevel: 0  
      ).sum.sin.sum * 0.5,  
      62,  
      24  
    ).tanh * gain;  
  }
});
)
