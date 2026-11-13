// Build-time only: config.ts hands each landing page just its own locale's copy.
import en from './en.js'
import es from './es.js'
import pt from './pt.js'
import fr from './fr.js'
import de from './de.js'
import id from './id.js'
import nl from './nl.js'
import tr from './tr.js'
import zh from './zh.js'
import zhTW from './zh-TW.js'
import ja from './ja.js'
import ro from './ro.js'
import it from './it.js'
import pl from './pl.js'
import vi from './vi.js'

export const MESSAGES = { en, es, pt, fr, de, id, nl, tr, zh, 'zh-TW': zhTW, ja, ro, it, pl, vi }
