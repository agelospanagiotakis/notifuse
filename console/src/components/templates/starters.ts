import type { EmailBlock, MJMLComponentType } from '../email_builder/types'

/**
 * Starter templates for the VISUAL editor.
 *
 * Why this file exists: every template in the kois-optics workspace is an imported Mailchimp
 * campaign, and those open in code mode — a 150-200 KB HTML document with the layout hard-coded.
 * A non-technical user cannot build the next campaign from one of those, so the platform had no
 * usable starting point for the people who actually send the mail.
 *
 * These are the same brand, expressed as blocks: the wordmark at a fixed width, the black
 * announcement band, the beige accent, the product rows and the contact/unsubscribe footer that
 * every campaign since 2024 carries. Everything below is editable in the visual editor — nothing
 * here is a picture of a layout.
 *
 * The values are taken from the campaigns themselves, not invented: the wordmark is the file the
 * kois-optics site serves, the palette is what the last two years of campaigns use (#111111 bands,
 * #FAFAFA page, #E8E1D5 accent, #333333 / #656565 text), and the footer contact block is copied
 * from the September 2026 issue.
 *
 * Editing rules that keep these safe:
 *  - `id` must be unique inside one tree and non-empty; readable ids are used so a designer
 *    debugging a template can tell what a block is.
 *  - Liquid variables are the platform's own (`{{ contact.first_name }}`, `{{ unsubscribe_url }}`,
 *    `{{ notification_center_url }}`), never Mailchimp's `*|FNAME|*`.
 *  - No absolute pixel widths on sections: MJML wants the section to size itself.
 */

/** Kois Optics brand constants, in one place so the four starters cannot drift apart. */
export const KOIS_BRAND = {
  logo: 'https://www.kois-optics.gr/themes/custom/twbase/img/logo.png',
  site: 'https://www.kois-optics.gr',
  instagram: 'https://www.instagram.com/koisoptics/',
  facebook: 'https://www.facebook.com/koisopticsgr',
  youtube: 'https://www.youtube.com/user/STAVROS1960KOIS',
  phone: '22810 83810',
  email: 'info@kois-optics.com',
  colors: {
    ink: '#111111',
    paper: '#FAFAFA',
    body: '#FFFFFF',
    text: '#333333',
    muted: '#656565',
    accent: '#E8E1D5',
    rule: '#E5E5E5'
  },
  font: 'Arial, Helvetica, sans-serif'
} as const

const C = KOIS_BRAND.colors

let seq = 0
/** Unique, readable block id scoped to the starter being built. */
const uid = (prefix: string) => `${prefix}-${++seq}`

type Attrs = Record<string, unknown>

function block(
  type: MJMLComponentType,
  prefix: string,
  attributes: Attrs = {},
  children: EmailBlock[] = [],
  content?: string
): EmailBlock {
  const b: Record<string, unknown> = { id: uid(prefix), type, attributes }
  if (children.length) b.children = children
  if (content !== undefined) b.content = content
  return b as unknown as EmailBlock
}

const section = (prefix: string, attributes: Attrs, children: EmailBlock[]) =>
  block('mj-section', prefix, attributes, children)

const column = (prefix: string, attributes: Attrs, children: EmailBlock[]) =>
  block('mj-column', prefix, attributes, children)

const text = (prefix: string, html: string, attributes: Attrs = {}) =>
  block('mj-text', prefix, { paddingTop: '8px', paddingBottom: '8px', ...attributes }, [], html)

const button = (prefix: string, label: string, href: string, attributes: Attrs = {}) =>
  block(
    'mj-button',
    prefix,
    {
      href,
      backgroundColor: C.ink,
      color: '#FFFFFF',
      borderRadius: '2px',
      fontSize: '14px',
      fontWeight: 'bold',
      paddingTop: '18px',
      paddingBottom: '18px',
      ...attributes
    },
    [],
    label
  )

const image = (prefix: string, src: string, alt: string, attributes: Attrs = {}) =>
  block('mj-image', prefix, { src, alt, paddingTop: '0px', paddingBottom: '0px', ...attributes })

const divider = (prefix: string) =>
  block('mj-divider', prefix, {
    borderColor: C.rule,
    borderStyle: 'solid',
    borderWidth: '1px',
    paddingTop: '20px',
    paddingBottom: '20px',
    width: '100%'
  })

/** The head every starter shares: fonts, button/ image defaults, title and inbox preview. */
function head(title: string): EmailBlock {
  return block('mj-head', 'head', {}, [
    block('mj-attributes', 'attrs', {}, [
      block('mj-text', 'attrs-text', {
        fontFamily: KOIS_BRAND.font,
        fontSize: '16px',
        color: C.text,
        lineHeight: '1.7',
        align: 'left',
        paddingTop: '6px',
        paddingBottom: '6px',
        paddingLeft: '0px',
        paddingRight: '0px'
      }),
      block('mj-button', 'attrs-button', {
        fontFamily: KOIS_BRAND.font,
        fontSize: '14px',
        fontWeight: 'bold',
        color: '#FFFFFF',
        backgroundColor: C.ink,
        borderRadius: '2px',
        align: 'center',
        letterSpacing: '0.06em'
      }),
      block('mj-image', 'attrs-image', {
        containerBackgroundColor: 'transparent',
        align: 'center',
        paddingTop: '0px',
        paddingBottom: '0px',
        paddingLeft: '0px',
        paddingRight: '0px'
      }),
      block('mj-section', 'attrs-section', {
        backgroundColor: C.body,
        paddingTop: '0px',
        paddingBottom: '0px',
        paddingLeft: '0px',
        paddingRight: '0px'
      })
    ]),
    block('mj-title', 'title', {}, [], title),
    block('mj-preview', 'preview', {}, [], '{{ subject_preview }}')
  ])
}

/** The wordmark, centred, linking home. Present in every campaign since 2024. */
const logoSection = () =>
  section('sec-logo', { backgroundColor: C.body, paddingTop: '28px', paddingBottom: '20px' }, [
    column('col-logo', {}, [
      image('img-logo', KOIS_BRAND.logo, 'Kois Optics', {
        width: '230px',
        href: KOIS_BRAND.site
      })
    ])
  ])

/** A black band with a headline — the device the campaigns use to mark the lead story. */
const inkBand = (eyebrow: string, headline: string, sub?: string) =>
  section('sec-ink', { backgroundColor: C.ink, paddingTop: '34px', paddingBottom: '34px' }, [
    column('col-ink', { paddingLeft: '32px', paddingRight: '32px' }, [
      text('txt-ink-eyebrow', eyebrow, {
        color: '#B9B9B9',
        fontSize: '12px',
        align: 'center',
        paddingBottom: '6px'
      }),
      text('txt-ink-headline', headline, {
        color: '#FFFFFF',
        fontSize: '24px',
        lineHeight: '1.35',
        fontWeight: 'bold',
        align: 'center',
        paddingTop: '0px',
        paddingBottom: sub ? '10px' : '0px'
      }),
      ...(sub
        ? [text('txt-ink-sub', sub, { color: '#D6D6D6', fontSize: '15px', align: 'center', paddingTop: '0px' })]
        : [])
    ])
  ])

/** Social row: the four icons every campaign carries. */
const socialRow = () =>
  section('sec-social', { backgroundColor: C.body, paddingTop: '18px', paddingBottom: '18px' }, [
    column('col-social', {}, [
      block('mj-social', 'social', { align: 'center', iconSize: '22px', iconHeight: '22px', mode: 'horizontal' }, [
        block('mj-social-element', 'soc-ig', { name: 'instagram', href: KOIS_BRAND.instagram }),
        block('mj-social-element', 'soc-fb', { name: 'facebook-noshare', href: KOIS_BRAND.facebook }),
        block('mj-social-element', 'soc-yt', { name: 'youtube', href: KOIS_BRAND.youtube }),
        block('mj-social-element', 'soc-web', { name: 'web', href: KOIS_BRAND.site })
      ])
    ])
  ])

/** Contact block: the same three lines the September 2026 issue closes with. */
const contactSection = () =>
  section('sec-contact', { backgroundColor: C.accent, paddingTop: '26px', paddingBottom: '26px' }, [
    column('col-contact', { paddingLeft: '24px', paddingRight: '24px' }, [
      text('txt-contact-title', 'Μείνετε σε επαφή', {
        fontSize: '13px',
        color: C.muted,
        align: 'center',
        paddingBottom: '10px'
      }),
      text(
        'txt-contact-body',
        `<p style="margin:0;line-height:1.9;">Σταύρος Κόης · Σύρος<br />
        ☎️ ${KOIS_BRAND.phone}<br />
        ✉️ <a href="mailto:${KOIS_BRAND.email}" style="color:${C.text};">${KOIS_BRAND.email}</a><br />
        <a href="${KOIS_BRAND.site}" style="color:${C.text};">kois-optics.gr</a></p>`,
        { align: 'center', fontSize: '15px' }
      )
    ])
  ])

/** The compliance footer: who sent it, how to leave. */
const legalFooter = () =>
  section('sec-legal', { backgroundColor: C.paper, paddingTop: '22px', paddingBottom: '26px' }, [
    column('col-legal', { paddingLeft: '24px', paddingRight: '24px' }, [
      text(
        'txt-legal',
        `<p style="margin:0;font-size:12px;line-height:1.8;color:${C.muted};text-align:center;">
        Λαμβάνετε αυτό το email στο {{ contact.email }}.<br />
        <a href="{{ unsubscribe_url }}" style="color:${C.muted};">Διαγραφή από τη λίστα</a> ·
        <a href="{{ notification_center_url }}" style="color:${C.muted};">Επιλογές επικοινωνίας</a>
        </p>`,
        { fontSize: '12px' }
      )
    ])
  ])

const body = (children: EmailBlock[]) =>
  block('mj-body', 'body', { backgroundColor: C.paper, width: '600px' }, children)

/** A product card: image above caption above a "Δείτε το" link, as the campaigns arrange them. */
const productCard = (title: string, note: string) =>
  column('col-product', { paddingLeft: '8px', paddingRight: '8px' }, [
    image('img-product', 'https://placehold.co/560x420/F2F2F2/9A9A9A?text=%CE%A0%CF%81%CE%BF%CF%8A%CF%8C%CE%BD', title),
    text('txt-product-title', title, {
      fontSize: '15px',
      fontWeight: 'bold',
      align: 'center',
      paddingTop: '12px',
      paddingBottom: '2px'
    }),
    text('txt-product-note', note, {
      fontSize: '13px',
      color: C.muted,
      align: 'center',
      paddingTop: '0px'
    })
  ])

export interface TemplateStarter {
  id: string
  /** Shown in the picker. Greek, because the people using it work in Greek. */
  name: string
  /** One line on when to reach for this one. */
  description: string
  /** Category offered when this starter is chosen. */
  category: 'marketing' | 'transactional'
  tree: EmailBlock
}

const offerStarter = (): EmailBlock => {
  seq = 0
  return block('mjml', 'mjml', {}, [
    head('Προσφορά · Kois Optics'),
    body([
      // The wordmark leads every campaign this client sends, so it leads the offer too; the
      // black band underneath is what makes an offer read as an offer.
      logoSection(),
      inkBand('ΠΡΟΣΦΟΡΑ', 'Τίτλος προσφοράς σε δύο γραμμές', 'Μία σύντομη γραμμή που εξηγεί την προσφορά'),
      section('sec-hero', { backgroundColor: C.body, paddingTop: '0px', paddingBottom: '0px' }, [
        column('col-hero', {}, [
          image('img-hero', 'https://placehold.co/560x340/F2F2F2/9A9A9A?text=%CE%95%CE%B9%CE%BA%CF%8C%CE%BD%CE%B1', 'Προσφορά')
        ])
      ]),
      section('sec-intro', { backgroundColor: C.body, paddingTop: '26px', paddingBottom: '8px' }, [
        column('col-intro', { paddingLeft: '24px', paddingRight: '24px' }, [
          text(
            'txt-intro',
            `<p>Γράψτε εδώ δύο έως τρεις γραμμές για την προσφορά. Τι περιλαμβάνει, μέχρι πότε ισχύει, και γιατί αξίζει να την προσέξει ο αναγνώστης.</p>`
          )
        ])
      ]),
      section('sec-products', { backgroundColor: C.body, paddingTop: '10px', paddingBottom: '10px' }, [
        productCard('Μοντέλο πρώτο', 'Σύντομη περιγραφή'),
        productCard('Μοντέλο δεύτερο', 'Σύντομη περιγραφή')
      ]),
      section('sec-cta', { backgroundColor: C.body, paddingTop: '14px', paddingBottom: '26px' }, [
        column('col-cta', {}, [
          button('btn-cta', 'ΔΕΙΤΕ ΤΗΝ ΠΡΟΣΦΟΡΑ', `${KOIS_BRAND.site}/el/prosfores`)
        ])
      ]),
      socialRow(),
      contactSection(),
      legalFooter()
    ])
  ])
}

const collectionStarter = (): EmailBlock => {
  seq = 0
  return block('mjml', 'mjml', {}, [
    head('Νέα συλλογή · Kois Optics'),
    body([
      logoSection(),
      section('sec-hero', { backgroundColor: C.body }, [
        column('col-hero', {}, [
          image('img-hero', 'https://placehold.co/600x380/F2F2F2/9A9A9A?text=%CE%9D%CE%AD%CE%B1+%CF%83%CF%85%CE%BB%CE%BB%CE%BF%CE%B3%CE%AE', 'Νέα συλλογή')
        ])
      ]),
      section('sec-title', { backgroundColor: C.body, paddingTop: '24px', paddingBottom: '4px' }, [
        column('col-title', { paddingLeft: '24px', paddingRight: '24px' }, [
          text('txt-kicker', 'ΝΕΑ ΣΥΛΛΟΓΗ', { fontSize: '12px', color: C.muted, align: 'center' }),
          text('txt-title', 'Ο τίτλος της συλλογής', {
            fontSize: '26px',
            fontWeight: 'bold',
            align: 'center',
            paddingTop: '4px'
          }),
          text(
            'txt-lede',
            `<p style="text-align:center;">Ένα σύντομο εισαγωγικό κείμενο — δύο γραμμές για την έμπνευση πίσω από τη συλλογή.</p>`
          )
        ])
      ]),
      section('sec-grid', { backgroundColor: C.body, paddingTop: '8px', paddingBottom: '8px' }, [
        productCard('Μοντέλο πρώτο', 'Οίκος'),
        productCard('Μοντέλο δεύτερο', 'Οίκος'),
        productCard('Μοντέλο τρίτο', 'Οίκος')
      ]),
      section('sec-cta', { backgroundColor: C.body, paddingTop: '16px', paddingBottom: '26px' }, [
        column('col-cta', {}, [button('btn-cta', 'ΔΕΙΤΕ ΤΗ ΣΥΛΛΟΓΗ', KOIS_BRAND.site)])
      ]),
      socialRow(),
      contactSection(),
      legalFooter()
    ])
  ])
}

const storyStarter = (): EmailBlock => {
  seq = 0
  return block('mjml', 'mjml', {}, [
    head('Ενημερωτικό · Kois Optics'),
    body([
      logoSection(),
      inkBand('ΤΟ ΘΕΜΑ', 'Ο τίτλος της ιστορίας'),
      section('sec-lead', { backgroundColor: C.body, paddingTop: '26px' }, [
        column('col-lead', { paddingLeft: '24px', paddingRight: '24px' }, [
          text(
            'txt-lead',
            `<p>Η πρώτη παράγραφος. Γράψτε εδώ το κύριο νέο, με τρόπο που να διαβάζεται και σε κινητό: μικρές προτάσεις, καμία υπερβολή.</p>`
          )
        ])
      ]),
      section('sec-image', { backgroundColor: C.body }, [
        column('col-image', {}, [
          image('img-story', 'https://placehold.co/560x340/F2F2F2/9A9A9A?text=%CE%A6%CF%89%CF%84%CE%BF%CE%B3%CF%81%CE%B1%CF%86%CE%AF%CE%B1', 'Ιστορία')
        ])
      ]),
      section('sec-copy', { backgroundColor: C.body, paddingTop: '18px', paddingBottom: '6px' }, [
        column('col-copy', { paddingLeft: '24px', paddingRight: '24px' }, [
          text(
            'txt-copy-1',
            `<p>Δεύτερη παράγραφος: το πλαίσιο. Ποιοι συμμετείχαν, πού έγινε, τι σημαίνει.</p>`
          ),
          text(
            'txt-copy-2',
            `<p>Τρίτη παράγραφος: η λεπτομέρεια που κάνει τη διαφορά — μια τιμή, μια ημερομηνία, ένα υλικό.</p>`
          )
        ])
      ]),
      section('sec-quote', { backgroundColor: C.accent, paddingTop: '22px', paddingBottom: '22px' }, [
        column('col-quote', { paddingLeft: '28px', paddingRight: '28px' }, [
          text(
            'txt-quote',
            `<p style="text-align:center;font-style:italic;">«Μια φράση που συνοψίζει την ιστορία, και θα ήθελε κανείς να τη θυμάται.»</p>`,
            { fontSize: '17px' }
          )
        ])
      ]),
      section('sec-cta', { backgroundColor: C.body, paddingTop: '20px', paddingBottom: '24px' }, [
        column('col-cta', {}, [button('btn-cta', 'ΔΙΑΒΑΣΤΕ ΠΕΡΙΣΣΟΤΕΡΑ', KOIS_BRAND.site)])
      ]),
      section('sec-sign', { backgroundColor: C.body }, [
        column('col-sign', { paddingLeft: '24px', paddingRight: '24px' }, [
          divider('div-sign'),
          text('txt-sign', '<p>Με εκτίμηση,<br /><strong>Ομάδα Kois Optics</strong></p>')
        ])
      ]),
      socialRow(),
      contactSection(),
      legalFooter()
    ])
  ])
}

const welcomeStarter = (): EmailBlock => {
  seq = 0
  return block('mjml', 'mjml', {}, [
    head('Καλώς ήρθατε · Kois Optics'),
    body([
      logoSection(),
      section('sec-welcome', { backgroundColor: C.body, paddingTop: '16px', paddingBottom: '6px' }, [
        column('col-welcome', { paddingLeft: '28px', paddingRight: '28px' }, [
          text('txt-hi', 'Καλώς ήρθατε, {{ contact.first_name }}', {
            fontSize: '24px',
            fontWeight: 'bold',
            align: 'center',
            paddingBottom: '10px'
          }),
          text(
            'txt-body',
            `<p style="text-align:center;">Σας ευχαριστούμε που γραφτήκατε στο newsletter του Kois Optics. Από εδώ και πέρα θα λαμβάνετε τις νέες συλλογές, τις προσφορές και τις ιστορίες μας από τη Σύρο — λίγες φορές τον μήνα, ποτέ άλλοτε.</p>`
          )
        ])
      ]),
      section('sec-image', { backgroundColor: C.body, paddingTop: '10px' }, [
        column('col-image', {}, [
          image('img-welcome', 'https://placehold.co/560x300/F2F2F2/9A9A9A?text=Kois+Optics', 'Kois Optics')
        ])
      ]),
      section('sec-cta', { backgroundColor: C.body, paddingTop: '18px', paddingBottom: '22px' }, [
        column('col-cta', {}, [button('btn-cta', 'ΕΠΙΣΚΕΦΘΕΙΤΕ ΤΟ ΚΑΤΑΣΤΗΜΑ', KOIS_BRAND.site)])
      ]),
      socialRow(),
      contactSection(),
      legalFooter()
    ])
  ])
}

/**
 * The gallery, in the order it is shown. Kept small on purpose: four shapes cover the campaigns
 * this client actually sends (offer, collection, story, welcome), and a longer list is a menu
 * nobody reads.
 */
export const TEMPLATE_STARTERS: TemplateStarter[] = [
  {
    id: 'kois-offer',
    name: 'Προσφορά',
    description: 'Μαύρη κεφαλίδα, εικόνα, δύο προϊόντα και κουμπί — για προσφορές και εκπτώσεις.',
    category: 'marketing',
    tree: offerStarter()
  },
  {
    id: 'kois-collection',
    name: 'Νέα συλλογή',
    description: 'Λογότυπο, μεγάλη εικόνα και τρία προϊόντα σε σειρά — για νέες αφίξεις.',
    category: 'marketing',
    tree: collectionStarter()
  },
  {
    id: 'kois-story',
    name: 'Ενημερωτικό',
    description: 'Λογότυπο, κείμενο, εικόνα και αποσπάσματα — για ιστορίες και ανακοινώσεις.',
    category: 'marketing',
    tree: storyStarter()
  },
  {
    id: 'kois-welcome',
    name: 'Καλώς ήρθατε',
    description: 'Σύντομο μήνυμα καλωσορίσματος με ένα κουμπί — για την πρώτη επαφή.',
    category: 'transactional',
    tree: welcomeStarter()
  }
]
