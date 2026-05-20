import { useState } from 'react';
import HeroSection from './components/HeroSection';
import SocialProofSection from './components/SocialProofSection';
import AppDemoSection from './components/AppDemoSection';
import FeaturesSection from './components/FeaturesSection';
import Footer from './components/Footer';
import LoadingScreen from './components/LoadingScreen';
import WebApp from './components/WebApp';

function App() {
  const [isLoading, setIsLoading] = useState(true);
  const isWebApp = window.location.pathname.startsWith('/app');

  if (isWebApp) {
    return <WebApp />;
  }

  if (isLoading) {
    return <LoadingScreen onComplete={() => setIsLoading(false)} />;
  }

  return (
    <div className="w-full bg-background min-h-screen text-foreground overflow-x-hidden">
      <HeroSection />
      <SocialProofSection />
      <AppDemoSection />
      <FeaturesSection />
      <Footer />
    </div>
  );
}

export default App;
