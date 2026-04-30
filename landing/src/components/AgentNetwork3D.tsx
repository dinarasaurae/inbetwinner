import { useRef, useMemo } from 'react';
import { Canvas, useFrame } from '@react-three/fiber';
import { OrbitControls, Sphere, Line, Preload } from '@react-three/drei';
import { EffectComposer, Bloom, Noise } from '@react-three/postprocessing';
import * as THREE from 'three';

const ORBS = [
  { position: [-2, 1, -1], color: '#4E85BF', size: 0.6 },  // Nurturing (Blue)
  { position: [2, 0.5, 1], color: '#4E85BF', size: 0.5 },  // Scoring
  { position: [0, 2, 0], color: '#22c55e', size: 0.8 },    // Supervisor (Hot Green)
  { position: [-1.5, -1.5, 0], color: '#9d4edd', size: 0.4 }, // RAG (Purple)
  { position: [1.5, -1, -1], color: '#9d4edd', size: 0.5 },
];

const CONNECTIONS = [
  [0, 2], [1, 2], [3, 2], [4, 2], // All to Supervisor
  [0, 3], [1, 4], [3, 4]          // Inter-agent meshes
];

const NodeNetwork = () => {
  const groupRef = useRef<THREE.Group>(null);

  // Slowly rotate the entire network
  useFrame((state) => {
    if (groupRef.current) {
      groupRef.current.rotation.y = state.clock.getElapsedTime() * 0.15;
      groupRef.current.rotation.x = Math.sin(state.clock.getElapsedTime() * 0.1) * 0.1;
    }
  });

  // Calculate lines between orbs
  const lines = useMemo(() => {
    return CONNECTIONS.map((conn) => {
      const start = ORBS[conn[0]].position;
      const end = ORBS[conn[1]].position;
      return {
        id: `${conn[0]}-${conn[1]}`,
        points: [
          new THREE.Vector3(...start),
          new THREE.Vector3(...end),
        ],
      };
    });
  }, []);

  return (
    <group ref={groupRef}>
      {/* 5 Agent Orbs */}
      {ORBS.map((orb, i) => (
        <Sphere key={i} args={[orb.size, 32, 32]} position={new THREE.Vector3(...orb.position)}>
          <meshStandardMaterial 
            color={orb.color} 
            emissive={orb.color}
            emissiveIntensity={1.5}
            transparent
            opacity={0.9}
            roughness={0.2}
            metalness={0.8}
          />
        </Sphere>
      ))}

      {/* Laser Connections */}
      {lines.map((line) => (
        <Line
          key={line.id}
          points={line.points}
          color="#89AACC"
          lineWidth={2}
          transparent
          opacity={0.4}
        />
      ))}
      
      {/* Central Core Ambient Light */}
      <pointLight position={[0, 0, 0]} intensity={2} color="#ffffff" distance={5} />
    </group>
  );
};

export default function AgentNetwork3D() {
  return (
    <Canvas camera={{ position: [0, 0, 6], fov: 45 }}>
      <color attach="background" args={['#050508']} />
      <ambientLight intensity={0.5} />
      
      <NodeNetwork />

      <OrbitControls 
        enableZoom={false} 
        enablePan={false}
        autoRotate={true}
        autoRotateSpeed={0.5}
        maxPolarAngle={Math.PI / 1.5}
        minPolarAngle={Math.PI / 3}
      />
      
      <EffectComposer>
        <Bloom 
          luminanceThreshold={0.5} 
          mipmapBlur 
          intensity={1.2} 
        />
        <Noise opacity={0.03} />
      </EffectComposer>
      
      <Preload all />
    </Canvas>
  );
}
