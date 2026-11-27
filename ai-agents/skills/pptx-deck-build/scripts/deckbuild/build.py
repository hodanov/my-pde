from .drawing import fill_placeholder
from .patterns import PATTERNS, Context
from .template import attach_slide_number, layout_by_name, make_base_presentation


class DeckBuilder:
    def __init__(self, ctx):
        self.ctx = ctx
        self.prs = make_base_presentation()

    def new_slide(self, pattern, slide):
        slide_obj = self.prs.slides.add_slide(layout_by_name(self.prs, pattern.layout))
        fill_placeholder(slide_obj.shapes.title, [slide.headline], pattern.title_override_pt(self.ctx))
        if slide.notes:
            slide_obj.notes_slide.notes_text_frame.text = slide.notes
        if pattern.numbered:
            attach_slide_number(slide_obj)
        return slide_obj


def build_deck(deck, output):
    builder = DeckBuilder(Context(deck.mode, deck.base_dir))
    for slide in deck.slides():
        PATTERNS[slide.pattern].build(builder, slide)
    builder.prs.core_properties.title = deck.title
    builder.prs.save(output)
