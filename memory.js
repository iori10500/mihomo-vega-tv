import Core from './src/NativeCore';
export function measureMemory(role) {
  const read = () => {
    const sample = Core.memorySample();
    if (sample) console.info('TV memory '+role+' '+sample);
    return Boolean(sample);
  };
  if (!read()) return () => {};
  const timer=setInterval(read,5000);
  return () => clearInterval(timer);
}
