module.exports = {
  project: {kepler: {sourceDir: '.'}},
  dependency: {platforms: {kepler: {autolink: {
    MihomoCore: {
      libraryName: 'libmihomotv.so', provider: 'application',
      linkDynamic: true, components: [], turbomodules: ['Core'],
    },
  }}}},
};
