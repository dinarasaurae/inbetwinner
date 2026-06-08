import React, { useRef, useEffect } from 'react';

const brands = ["VK", "Telegram", "Zoho CRM", "Google"];

// Create seamless loop duplicate
const marqueeItems = [...brands, ...brands, ...brands];

const SocialProofSection: React.FC = () => {
  const videoRef = useRef<HTMLVideoElement>(null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    let animationFrameId: number;

    const updateOpacity = () => {
      if (!video) return;
      const { currentTime, duration } = video;
      let opacity = 1;

      // Ensure duration is valid before math
      if (duration > 0) {
        if (currentTime < 0.5) {
          // Fade in
          opacity = currentTime / 0.5;
        } else if (duration - currentTime < 0.5) {
          // Fade out
          opacity = Math.max(0, (duration - currentTime) / 0.5);
        }
      }
      
      video.style.opacity = opacity.toString();
      animationFrameId = requestAnimationFrame(updateOpacity);
    };

    const onEnded = () => {
      if (video) {
        video.style.opacity = '0';
        setTimeout(() => {
          video.currentTime = 0;
          video.play().catch(() => {});
        }, 100);
      }
    };

    video.addEventListener('ended', onEnded);
    video.addEventListener('play', () => {
      animationFrameId = requestAnimationFrame(updateOpacity);
    });

    // Handle initial state
    video.style.opacity = '0';

    return () => {
      video.removeEventListener('ended', onEnded);
      cancelAnimationFrame(animationFrameId);
    };
  }, []);

  return (
    <section className="relative w-full overflow-hidden min-h-[60vh] bg-background flex flex-col justify-end">
      {/* Background Video */}
      <video 
        ref={videoRef}
        src="https://d8j0ntlcm91z4.cloudfront.net/user_38xzZboKViGWJOttwIXH07lWA1P/hf_20260308_114720_3dabeb9e-2c39-4907-b747-bc3544e2d5b7.mp4"
        autoPlay 
        muted 
        playsInline 
        className="absolute inset-0 w-full h-full object-cover object-[center_40%] scale-[1.12] z-0"
        style={{ opacity: 0, transition: 'opacity 0.1s linear' }}
      />
      
      {/* Gradient Overlays */}
      <div className="absolute inset-0 bg-gradient-to-b from-background via-transparent to-background z-10"></div>
      <div className="absolute inset-0 bg-black/40 z-10 pointer-events-none"></div>

      {/* Content */}
      <div className="relative z-20 flex flex-col items-center pt-16 pb-24 px-4 gap-20 w-full">
        {/* Spacer for video visibility */}
        <div className="h-40 w-full" />
        
        {/* Logo Marquee Container */}
        <div className="max-w-5xl w-full mx-auto">
          <div className="w-full overflow-hidden relative">
             <div className="absolute top-0 bottom-0 left-0 w-16 bg-gradient-to-r from-background to-transparent z-10 pointer-events-none"></div>
             <div className="absolute top-0 bottom-0 right-0 w-16 bg-gradient-to-l from-background to-transparent z-10 pointer-events-none"></div>
             
             <div className="flex w-max animate-marquee items-center gap-16">
               {marqueeItems.map((brand, idx) => (
                 <div key={idx} className="flex items-center gap-4">
                   <div className="w-8 h-8 rounded-lg liquid-glass flex items-center justify-center text-xs font-bold text-foreground">
                     {brand[0]}
                   </div>
                   <div className="text-base font-semibold text-foreground">
                     {brand}
                   </div>
                 </div>
               ))}
             </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default SocialProofSection;
