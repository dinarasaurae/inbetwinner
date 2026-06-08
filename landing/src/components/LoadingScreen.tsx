import React, { useEffect, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';

const words = ["Score", "Orchestrate", "Automate"];

const LoadingScreen: React.FC<{ onComplete: () => void }> = ({ onComplete }) => {
  const [count, setCount] = useState(0);
  const [wordIndex, setWordIndex] = useState(0);

  useEffect(() => {
    const start = performance.now();
    const duration = 2700;

    const animateCount = (now: number) => {
      const elapsed = now - start;
      const progress = Math.min(elapsed / duration, 1);
      const currentCount = Math.floor(progress * 100);
      setCount(currentCount);

      if (progress < 1) {
        requestAnimationFrame(animateCount);
      } else {
        setTimeout(onComplete, 400); // 400ms delay on complete
      }
    };

    requestAnimationFrame(animateCount);

    const intvl = setInterval(() => {
      setWordIndex((prev) => (prev + 1) % words.length);
    }, 900);

    return () => clearInterval(intvl);
  }, [onComplete]);

  return (
    <div className="fixed inset-0 z-[9999] bg-bg flex flex-col justify-between p-8 font-display">
      <motion.div 
        className="text-xs text-muted uppercase tracking-[0.3em] font-body"
        initial={{ y: -20, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ duration: 0.8 }}
      >
        <img src="/inbetwin-logo.png" alt="inBeTwin" className="h-12 w-auto brightness-0 invert" />
      </motion.div>

      <div className="flex-1 flex items-center justify-center">
        <AnimatePresence mode="wait">
          <motion.div
            key={wordIndex}
            initial={{ y: 20, opacity: 0 }}
            animate={{ y: 0, opacity: 1 }}
            exit={{ y: -20, opacity: 0 }}
            transition={{ duration: 0.4 }}
            className="text-4xl md:text-6xl lg:text-7xl italic text-text-primary/80 font-display"
          >
            {words[wordIndex]}
          </motion.div>
        </AnimatePresence>
      </div>

      <div className="flex flex-col gap-4">
        <div className="text-right text-6xl md:text-8xl lg:text-9xl font-display text-text-primary tabular-nums">
          {String(count).padStart(3, "0")}
        </div>
        <div className="h-[3px] bg-stroke/50 max-w-[200px] w-full ml-auto overflow-hidden relative">
          <div 
            className="absolute inset-y-0 left-0 accent-gradient w-full origin-left"
            style={{ 
              transform: `scaleX(${count / 100})`, 
              boxShadow: "0 0 8px rgba(137, 170, 204, 0.35)" 
            }}
          />
        </div>
      </div>
    </div>
  );
};

export default LoadingScreen;
