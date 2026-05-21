/** 开源仓库地址（与 git remote 一致） */
export const GITHUB_REPO_URL =
  typeof __ZENMIND_GITHUB_URL__ !== 'undefined'
    ? __ZENMIND_GITHUB_URL__
    : 'https://github.com/TechxTry/ZenMind'

/** 应用版本，构建时由根目录 VERSION 注入 */
export const APP_VERSION =
  typeof __ZENMIND_VERSION__ !== 'undefined' ? __ZENMIND_VERSION__ : 'dev'
