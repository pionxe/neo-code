import DefaultTheme from 'vitepress/theme'
import './custom.css'
import ArchitectureGrid from './components/ArchitectureGrid.vue'
import QuickStartCards from './components/QuickStartCards.vue'

export default {
  ...DefaultTheme,
  enhanceApp({ app }) {
    app.component('ArchitectureGrid', ArchitectureGrid)
    app.component('QuickStartCards', QuickStartCards)
  }
}
